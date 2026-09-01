// Package speechcontext builds bounded local Whisper decoder prompts.
package speechcontext

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Options controls prompt construction. A disabled builder always returns an
// empty prompt, preserving Voxi's historical transcription command exactly.
type Options struct {
	Enabled      bool
	PromptPrefix string
	MaxTerms     int
	MaxChars     int
	MaxTermChars int
}

// Sources are ordered by priority: explicit user terms win over the shipped
// static vocabulary, which wins over repository-derived metadata.
type Sources struct {
	Explicit   []string
	Static     []string
	Repository []string
}

// Build returns a short natural-language prompt suitable for voxtype's
// --initial-prompt. It never reads files or rewrites a transcript.
func Build(opts Options, sources Sources) string {
	if !opts.Enabled || opts.MaxTerms <= 0 || opts.MaxChars <= 0 || opts.MaxTermChars <= 0 {
		return ""
	}
	prefix := strings.TrimSpace(opts.PromptPrefix)
	if prefix == "" || utf8.RuneCountInString(prefix) >= opts.MaxChars {
		return ""
	}

	seen := make(map[string]struct{})
	terms := make([]string, 0, opts.MaxTerms)
	for _, group := range [][]string{sources.Explicit, sources.Static, sources.Repository} {
		for _, raw := range group {
			term, err := NormalizeTerm(raw, opts.MaxTermChars)
			if err != nil {
				continue
			}
			key := strings.ToLower(term)
			if _, exists := seen[key]; exists {
				continue
			}
			candidate := prefix + " " + strings.Join(append(append([]string(nil), terms...), term), ", ")
			if utf8.RuneCountInString(candidate) > opts.MaxChars {
				return render(prefix, terms)
			}
			seen[key] = struct{}{}
			terms = append(terms, term)
			if len(terms) == opts.MaxTerms {
				return render(prefix, terms)
			}
		}
	}
	return render(prefix, terms)
}

func render(prefix string, terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	return prefix + " " + strings.Join(terms, ", ")
}

// NormalizeTerm applies the same privacy and character rules used by Build.
// It is exported so persistent vocabulary management cannot drift from the
// decoder prompt sanitizer.
func NormalizeTerm(raw string, maxChars int) (string, error) {
	if maxChars <= 0 {
		return "", fmt.Errorf("vocabulary term limit must be positive")
	}
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("vocabulary term must not be empty")
	}

	// Keep only the basename if a caller accidentally supplies a path. This
	// prevents parent directories (which may contain usernames or secrets)
	// from entering the decoder prompt.
	raw = strings.ReplaceAll(raw, `\`, "/")
	if slash := strings.LastIndexByte(raw, '/'); slash >= 0 {
		raw = raw[slash+1:]
	}

	var cleaned strings.Builder
	hasLetterOrDigit := false
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			cleaned.WriteRune(r)
			hasLetterOrDigit = true
		case unicode.IsSpace(r), strings.ContainsRune("._+#@-", r):
			cleaned.WriteRune(r)
		default:
			cleaned.WriteRune(' ')
		}
	}
	term := strings.Join(strings.Fields(cleaned.String()), " ")
	if !hasLetterOrDigit || term == "" {
		return "", fmt.Errorf("vocabulary term must contain a letter or digit")
	}
	if utf8.RuneCountInString(term) > maxChars {
		return "", fmt.Errorf("vocabulary term must be at most %d characters", maxChars)
	}
	return term, nil
}

// ParseVocabulary reads a privacy-simple one-term-per-line vocabulary. Blank
// lines are ignored; all sanitization remains centralized in Build.
func ParseVocabulary(data []byte) []string {
	var terms []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if term := strings.TrimSpace(scanner.Text()); term != "" {
			terms = append(terms, term)
		}
	}
	return terms
}

// VocabularyPath is the opt-in user's local Voxi vocabulary file.
func VocabularyPath(home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "voxi", "vocabulary.txt")
}

// DiscoverRepositoryTerms returns only safe local metadata: the repository
// basename and basenames of the most recently modified tracked files. It does
// not inspect file contents, remotes, history, environment variables, or
// untracked paths. Failure simply means no repository terms are available.
func DiscoverRepositoryTerms(ctx context.Context, cwd string, fileLimit int) []string {
	if cwd == "" || fileLimit <= 0 {
		return nil
	}
	rootBytes, err := exec.CommandContext(ctx, "git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil
	}
	root := strings.TrimSpace(string(rootBytes))
	if root == "" {
		return nil
	}
	tracked, err := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--cached").Output()
	if err != nil {
		return []string{filepath.Base(root)}
	}
	type fileTerm struct {
		path  string
		name  string
		mtime int64
	}
	var files []fileTerm
	for _, pathBytes := range bytes.Split(tracked, []byte{0}) {
		path := string(pathBytes)
		if path == "" {
			continue
		}
		info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, fileTerm{path: path, name: filepath.Base(path), mtime: info.ModTime().UnixNano()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].mtime == files[j].mtime {
			return files[i].path < files[j].path
		}
		return files[i].mtime > files[j].mtime
	})
	terms := []string{filepath.Base(root)}
	for _, file := range files {
		terms = append(terms, file.name)
		if len(terms) > fileLimit {
			break
		}
	}
	return terms
}
