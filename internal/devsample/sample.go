// Package devsample implements a private, local-only developer sample
// recorder behind `voxi feedback sample record|list|play|remove`. It lets a
// developer build a small library of real microphone utterances, each paired
// with a manually corrected ground-truth transcript, for offline accuracy
// canaries (see issues 032, 040, 041). It is a separate, isolated dev-only
// capture path: it never touches eager dictation's typing output,
// hallucination filtering, or history.
package devsample

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ubunatic.com/voxi/internal/speechcontext"
)

// maxNameChars bounds a sample name the same way issue 038's vocabulary
// terms are bounded.
const maxNameChars = 64

// manifestFile is corpus.tsv-compatible with testdata/speech-context, so
// scripts/speech_context_bench can be pointed at SamplesDir unmodified (see
// the -corpus flag documented in scripts/speech_context_bench/main.go).
const manifestFile = "corpus.tsv"

// SamplesDir is the private per-user directory holding recorded dev samples
// and their manifest. Its contents are sensitive (private speech + manually
// typed text) and must never be swept into any Voxi-owned sync/backup
// tooling; see docs/DevSamples.md.
func SamplesDir(home string) string {
	return filepath.Join(home, ".config", "voxi", "samples")
}

// ManifestPath is the corpus.tsv-compatible manifest listing every sample.
func ManifestPath(home string) string { return filepath.Join(SamplesDir(home), manifestFile) }

// WAVPath is the private per-sample recording.
func WAVPath(home, name string) string { return filepath.Join(SamplesDir(home), name+".wav") }

// Sample is one recorded dev sample: a WAV recording paired with a manually
// corrected ground-truth transcript.
type Sample struct {
	Name      string
	WAVFile   string // basename, relative to SamplesDir
	Text      string
	Keyterms  string // corpus.tsv's 4th field: `|`-separated, may be empty
	Timestamp time.Time
}

// Preview returns text truncated to n runes for `list` output.
func (s Sample) Preview(n int) string {
	r := []rune(s.Text)
	if len(r) <= n {
		return s.Text
	}
	return string(r[:n]) + "..."
}

// SanitizeName mirrors and reuses issue 038's vocabulary term sanitizer
// (speechcontext.NormalizeTerm): it rejects empty, overlong, and
// control-character names. Path separators are rejected outright (rather
// than silently stripped) so a sample name always names exactly the file the
// user expects. The sanitized name is safe to use directly as a filename
// component and as the manifest key.
func SanitizeName(raw string) (string, error) {
	if strings.ContainsAny(raw, `/\`) {
		return "", fmt.Errorf("sample name must not contain path separators")
	}
	name, err := speechcontext.NormalizeTerm(raw, maxNameChars)
	if err != nil {
		return "", fmt.Errorf("sample name: %w", err)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("sample name must not be %q", name)
	}
	// NormalizeTerm preserves internal spaces (fine for spoken vocabulary
	// prompts); a sample name doubles as a filename component, so collapse
	// them to a single filesystem-safe token.
	name = strings.ReplaceAll(name, " ", "-")
	return name, nil
}

// ParseManifest parses a corpus.tsv-compatible manifest: entries are
// `<name>\t<wav file>\t<text>\t<keyterms>`, and blank/`#`-prefixed lines are
// comments (scripts/speech_context_bench already skips them). Dev sample
// timestamps ride along as `#ts <name> <RFC3339>` comment lines, so the file
// stays byte-for-byte compatible with the existing bench corpus reader.
func ParseManifest(data []byte) ([]Sample, error) {
	timestamps := map[string]time.Time{}
	var samples []Sample
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#ts "):
			fields := strings.Fields(strings.TrimPrefix(line, "#ts "))
			if len(fields) == 2 {
				if ts, err := time.Parse(time.RFC3339, fields[1]); err == nil {
					timestamps[fields[0]] = ts
				}
			}
		case strings.HasPrefix(line, "#"):
			continue
		default:
			fields := strings.SplitN(line, "\t", 4)
			if len(fields) < 3 {
				return nil, fmt.Errorf("malformed sample manifest line: %q", line)
			}
			keyterms := ""
			if len(fields) == 4 {
				keyterms = strings.TrimSpace(fields[3])
			}
			samples = append(samples, Sample{Name: fields[0], WAVFile: fields[1], Text: fields[2], Keyterms: keyterms})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for i, s := range samples {
		samples[i].Timestamp = timestamps[s.Name]
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].Name < samples[j].Name })
	return samples, nil
}

// FormatManifest renders samples back to the corpus.tsv-compatible format
// ParseManifest reads.
func FormatManifest(samples []Sample) []byte {
	sorted := append([]Sample(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var b strings.Builder
	b.WriteString("# id\twav file\texpected transcript\tkeyterms separated by |\n")
	b.WriteString("# Private local dev samples recorded via `voxi feedback sample record`.\n")
	b.WriteString("# corpus.tsv-compatible: point scripts/speech_context_bench -corpus at this\n")
	b.WriteString("# directory to include these real recordings in accuracy canaries.\n")
	for _, s := range sorted {
		fmt.Fprintf(&b, "#ts %s %s\n", s.Name, s.Timestamp.UTC().Format(time.RFC3339))
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", s.Name, s.WAVFile, sanitizeText(s.Text), sanitizeText(s.Keyterms))
	}
	return []byte(b.String())
}

func sanitizeText(text string) string {
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.TrimSpace(text)
}

// LoadManifest reads the manifest, treating a missing file as an empty
// sample set.
func LoadManifest(home string) ([]Sample, error) {
	data, err := os.ReadFile(ManifestPath(home))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sample manifest: %w", err)
	}
	return ParseManifest(data)
}

// SaveManifest atomically writes the manifest with private permissions.
func SaveManifest(home string, samples []Sample) error {
	dir := SamplesDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create samples directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure samples directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".corpus-*")
	if err != nil {
		return fmt.Errorf("create sample manifest file: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err == nil {
		_, err = f.Write(FormatManifest(samples))
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write sample manifest file: %w", err)
	}
	path := ManifestPath(home)
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace sample manifest file: %w", err)
	}
	return os.Chmod(path, 0o600)
}

// Find returns the sample named name, if present.
func Find(samples []Sample, name string) (Sample, bool) {
	for _, s := range samples {
		if s.Name == name {
			return s, true
		}
	}
	return Sample{}, false
}

// Upsert adds sample, replacing any existing entry with the same name.
func Upsert(samples []Sample, sample Sample) []Sample {
	out := make([]Sample, 0, len(samples)+1)
	replaced := false
	for _, s := range samples {
		if s.Name == sample.Name {
			out = append(out, sample)
			replaced = true
			continue
		}
		out = append(out, s)
	}
	if !replaced {
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// RemoveEntry removes the sample named name, reporting whether it was found.
func RemoveEntry(samples []Sample, name string) ([]Sample, bool) {
	out := make([]Sample, 0, len(samples))
	found := false
	for _, s := range samples {
		if s.Name == name {
			found = true
			continue
		}
		out = append(out, s)
	}
	return out, found
}
