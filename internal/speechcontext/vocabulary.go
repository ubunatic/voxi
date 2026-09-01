package speechcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadVocabulary reads and normalizes the persistent user vocabulary. A
// missing file represents an empty vocabulary.
func LoadVocabulary(path string, maxTermChars int) ([]string, error) {
	if path == "" {
		return nil, fmt.Errorf("vocabulary path is unavailable: HOME is not set")
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read vocabulary: %w", err)
	}
	terms, err := normalizeVocabulary(ParseVocabulary(data), maxTermChars)
	if err != nil {
		return nil, fmt.Errorf("invalid stored vocabulary: %w", err)
	}
	return terms, nil
}

// SaveVocabulary atomically writes one normalized term per line. The Voxi
// configuration directory and vocabulary file are private to the user.
func SaveVocabulary(path string, terms []string, maxTermChars int) error {
	if path == "" {
		return fmt.Errorf("vocabulary path is unavailable: HOME is not set")
	}
	normalized, err := normalizeVocabulary(terms, maxTermChars)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create vocabulary directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure vocabulary directory: %w", err)
	}

	f, err := os.CreateTemp(dir, ".vocabulary-*")
	if err != nil {
		return fmt.Errorf("create vocabulary file: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err == nil {
		content := ""
		if len(normalized) > 0 {
			content = strings.Join(normalized, "\n") + "\n"
		}
		_, err = f.WriteString(content)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write vocabulary file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace vocabulary file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure vocabulary file: %w", err)
	}
	return nil
}

// AddVocabulary adds one normalized term, rejecting case-insensitive
// duplicates. The returned term is the exact value to display to the user.
func AddVocabulary(terms []string, raw string, maxTermChars int) ([]string, string, error) {
	term, err := NormalizeTerm(raw, maxTermChars)
	if err != nil {
		return terms, "", err
	}
	normalized, err := normalizeVocabulary(terms, maxTermChars)
	if err != nil {
		return terms, "", err
	}
	for _, existing := range normalized {
		if strings.EqualFold(existing, term) {
			return terms, "", fmt.Errorf("vocabulary term %q is already present", term)
		}
	}
	normalized = append(normalized, term)
	sortVocabulary(normalized)
	return normalized, term, nil
}

// RemoveVocabulary removes one term using case-insensitive exact matching
// after applying the prompt sanitizer to the requested term.
func RemoveVocabulary(terms []string, raw string, maxTermChars int) ([]string, string, error) {
	term, err := NormalizeTerm(raw, maxTermChars)
	if err != nil {
		return terms, "", err
	}
	normalized, err := normalizeVocabulary(terms, maxTermChars)
	if err != nil {
		return terms, "", err
	}
	for i, existing := range normalized {
		if strings.EqualFold(existing, term) {
			removed := existing
			normalized = append(normalized[:i], normalized[i+1:]...)
			return normalized, removed, nil
		}
	}
	return terms, "", fmt.Errorf("vocabulary term %q was not found", term)
}

func normalizeVocabulary(terms []string, maxTermChars int) ([]string, error) {
	seen := make(map[string]struct{}, len(terms))
	normalized := make([]string, 0, len(terms))
	for _, raw := range terms {
		term, err := NormalizeTerm(raw, maxTermChars)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(term)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, term)
	}
	sortVocabulary(normalized)
	return normalized, nil
}

func sortVocabulary(terms []string) {
	sort.Slice(terms, func(i, j int) bool {
		left, right := strings.ToLower(terms[i]), strings.ToLower(terms[j])
		if left == right {
			return terms[i] < terms[j]
		}
		return left < right
	})
}
