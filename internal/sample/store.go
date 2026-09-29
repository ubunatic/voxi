// Package sample implements the purpose-separated persistent sample store.
package sample

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Purpose string

const (
	Dictation Purpose = "dictation"
	Noise     Purpose = "noise"
	Voice     Purpose = "voice"
)

var purposes = map[Purpose]bool{Dictation: true, Noise: true, Voice: true}
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// Sample is the JSON sidecar associated with an audio file named <ID>.<ext>.
type Sample struct {
	ID         string     `json:"id"`
	Transcript string     `json:"transcript"`
	Keyterms   []string   `json:"keyterms,omitempty"`
	Created    time.Time  `json:"created"`
	Source     string     `json:"source"`
	Consent    *time.Time `json:"consent,omitempty"`
	Purpose    Purpose    `json:"-"`
	Audio      string     `json:"-"`
}

// Store operates on a private store by default; OpenAt selects a public store mode.
type Store struct {
	root              string
	dirMode, fileMode os.FileMode
}

func Root(dataHome string) string {
	if dataHome == "" {
		dataHome = os.Getenv("XDG_DATA_HOME")
	}
	if dataHome == "" {
		home, _ := os.UserHomeDir()
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "voxi", "samples")
}

func Open(root string) (*Store, error)       { return open(root, 0o700, 0o600) }
func OpenPublic(root string) (*Store, error) { return open(root, 0o755, 0o644) }
func open(root string, dm, fm os.FileMode) (*Store, error) {
	if root == "" {
		return nil, errors.New("sample store root is empty")
	}
	if err := os.MkdirAll(root, dm); err != nil {
		return nil, fmt.Errorf("create sample store: %w", err)
	}
	if err := os.Chmod(root, dm); err != nil {
		return nil, fmt.Errorf("secure sample store: %w", err)
	}
	return &Store{root: root, dirMode: dm, fileMode: fm}, nil
}

func (s *Store) Root() string { return s.root }

// AudioPath returns the filesystem path of x's audio file.
func (s *Store) AudioPath(x Sample) string { return filepath.Join(s.dir(x.Purpose), x.Audio) }

// Put copies audio into the store and writes its sidecar.
func (s *Store) Put(x Sample, source string) error {
	if !validID(x.ID) {
		return fmt.Errorf("invalid sample id %q", x.ID)
	}
	if !validPurpose(x.Purpose) {
		return fmt.Errorf("invalid sample purpose %q", x.Purpose)
	}
	if _, err := s.Get(x.ID); err == nil {
		return fmt.Errorf("sample id %q already exists", x.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if filepath.Ext(source) == "" {
		return fmt.Errorf("sample audio %q has no extension", source)
	}
	if err := os.MkdirAll(s.dir(x.Purpose), s.dirMode); err != nil {
		return err
	}
	if err := os.Chmod(s.dir(x.Purpose), s.dirMode); err != nil {
		return err
	}
	x.Audio = x.ID + filepath.Ext(source)
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err = os.WriteFile(s.AudioPath(x), data, s.fileMode); err != nil {
		return err
	}
	if err = os.Chmod(s.AudioPath(x), s.fileMode); err != nil {
		_ = os.Remove(s.AudioPath(x))
		return err
	}
	if err = s.Add(x); err != nil {
		_ = os.Remove(s.AudioPath(x))
		return err
	}
	return nil
}

// UpdateTranscript replaces a sample's transcript.
func (s *Store) UpdateTranscript(id, transcript string) error {
	if strings.TrimSpace(transcript) == "" {
		return errors.New("sample transcript must not be empty")
	}
	x, err := s.Get(id)
	if err != nil {
		return err
	}
	x.Transcript = strings.TrimSpace(transcript)
	return s.write(x)
}

// Has reports whether id exists in the store.
func (s *Store) Has(id string) (bool, error) {
	_, err := s.Get(id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
func validID(id string) bool          { return idPattern.MatchString(id) }
func validPurpose(p Purpose) bool     { return purposes[p] }
func (s *Store) dir(p Purpose) string { return filepath.Join(s.root, string(p)) }

func (s *Store) List(filter ...Purpose) ([]Sample, error) {
	ps := []Purpose{Dictation, Noise, Voice}
	if len(filter) > 0 {
		ps = append([]Purpose(nil), filter...)
	}
	var out []Sample
	var listErr error
	for _, p := range ps {
		if !validPurpose(p) {
			return nil, fmt.Errorf("invalid sample purpose %q", p)
		}
		entries, err := os.ReadDir(s.dir(p))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
				continue
			}
			x, err := s.readSidecar(p, strings.TrimSuffix(ent.Name(), ".json"))
			if err != nil {
				listErr = errors.Join(listErr, err)
				continue
			}
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, listErr
}

func (s *Store) readSidecar(p Purpose, id string) (Sample, error) {
	data, err := os.ReadFile(filepath.Join(s.dir(p), id+".json"))
	if err != nil {
		return Sample{}, err
	}
	var x Sample
	if err = json.Unmarshal(data, &x); err != nil {
		return Sample{}, fmt.Errorf("%s/%s.json: %w", p, id, err)
	}
	if !validID(x.ID) || x.ID != id {
		return Sample{}, fmt.Errorf("%s/%s.json: invalid or mismatched id", p, id)
	}
	x.Purpose = p
	files, err := filepath.Glob(filepath.Join(s.dir(p), id+".*"))
	if err != nil {
		return Sample{}, err
	}
	for _, f := range files {
		if filepath.Ext(f) != ".json" {
			x.Audio = filepath.Base(f)
			break
		}
	}
	return x, nil
}

func (s *Store) Get(id string) (Sample, error) {
	if !validID(id) {
		return Sample{}, fmt.Errorf("invalid sample id %q", id)
	}
	for _, p := range []Purpose{Dictation, Noise, Voice} {
		x, err := s.readSidecar(p, id)
		if err == nil {
			return x, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Sample{}, err
		}
	}
	return Sample{}, os.ErrNotExist
}

// Add writes metadata atomically. Audio bytes must be placed by the caller before Add.
func (s *Store) Add(x Sample) error {
	if !validID(x.ID) {
		return fmt.Errorf("invalid sample id %q", x.ID)
	}
	if !validPurpose(x.Purpose) {
		return fmt.Errorf("invalid sample purpose %q", x.Purpose)
	}
	if _, err := s.Get(x.ID); err == nil {
		return fmt.Errorf("sample id %q already exists", x.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.write(x)
}

func (s *Store) write(x Sample) error {
	dir := s.dir(x.Purpose)
	if err := os.MkdirAll(dir, s.dirMode); err != nil {
		return err
	}
	if err := os.Chmod(dir, s.dirMode); err != nil {
		return err
	}
	x.Purpose = ""
	x.Audio = ""
	b, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	f, err := os.CreateTemp(dir, ".sample-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(s.fileMode); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, filepath.Join(dir, x.ID+".json")); err != nil {
		return err
	}
	return os.Chmod(filepath.Join(dir, x.ID+".json"), s.fileMode)
}

func (s *Store) Delete(id string) error {
	x, err := s.Get(id)
	if err != nil {
		return err
	}
	dir := s.dir(x.Purpose)
	if x.Audio != "" {
		if err = os.Remove(filepath.Join(dir, x.Audio)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Remove(filepath.Join(dir, id+".json"))
}

func (s *Store) Move(id string, p Purpose) error {
	if !validPurpose(p) {
		return fmt.Errorf("invalid sample purpose %q", p)
	}
	x, err := s.Get(id)
	if err != nil {
		return err
	}
	if x.Purpose == p {
		return nil
	}
	if p == Voice && x.Consent == nil {
		return errors.New("moving into voice requires own-voice consent")
	}
	old := s.dir(x.Purpose)
	target := s.dir(p)
	if err = os.MkdirAll(target, s.dirMode); err != nil {
		return err
	}
	if err = os.Chmod(target, s.dirMode); err != nil {
		return err
	}
	newSidecar := filepath.Join(target, id+".json")
	if _, err = os.Lstat(newSidecar); err == nil {
		return fmt.Errorf("sample id %q already exists in %s", id, p)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	x.Purpose = p
	if err = s.write(x); err != nil {
		return err
	}
	rollback := func(cause error, audioMoved bool) error {
		var rollbackErr error
		if audioMoved {
			rollbackErr = errors.Join(rollbackErr, os.Rename(filepath.Join(target, x.Audio), filepath.Join(old, x.Audio)))
		}
		rollbackErr = errors.Join(rollbackErr, os.Remove(newSidecar))
		return errors.Join(cause, rollbackErr)
	}
	audioMoved := false
	if x.Audio != "" {
		if err = os.Rename(filepath.Join(old, x.Audio), filepath.Join(target, x.Audio)); err != nil {
			return rollback(err, false)
		}
		audioMoved = true
	}
	if err = os.Remove(filepath.Join(old, id+".json")); err != nil {
		return rollback(err, audioMoved)
	}
	return nil
}

// LoadLegacyTSV reads a legacy corpus.tsv directory without modifying it.
func LoadLegacyTSV(dir string) ([]Sample, error) {
	f, err := os.Open(filepath.Join(dir, "corpus.tsv"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Sample
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		t := sc.Text()
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		fields := strings.SplitN(t, "\t", 4)
		if len(fields) < 3 {
			return nil, fmt.Errorf("corpus.tsv:%d: malformed row", line)
		}
		id := fields[0]
		if !validID(id) {
			return nil, fmt.Errorf("corpus.tsv:%d: invalid id %q", line, id)
		}
		x := Sample{ID: id, Audio: fields[1], Transcript: fields[2], Purpose: Dictation, Created: time.Time{}}
		if len(fields) == 4 && fields[3] != "" {
			x.Keyterms = strings.Split(fields[3], "|")
		}
		out = append(out, x)
	}
	return out, sc.Err()
}

// ExportTSV formats samples in the legacy four-column corpus format.
func ExportTSV(samples []Sample) []byte {
	x := append([]Sample(nil), samples...)
	sort.Slice(x, func(i, j int) bool { return x[i].ID < x[j].ID })
	var b strings.Builder
	b.WriteString("# id\twav file\texpected transcript\tkeyterms separated by |\n")
	for _, s := range x {
		audio := s.Audio
		if audio == "" {
			audio = s.ID + ".wav"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", s.ID, audio, sanitize(s.Transcript), sanitize(strings.Join(s.Keyterms, "|")))
	}
	return []byte(b.String())
}
func sanitize(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s))
}
