package feedback

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ubunatic.com/voxi/internal/devsample"
	"ubunatic.com/voxi/internal/speechcontext"
)

// Import area names, as accepted by ImportOptions.Only and printed in
// summary output. Order here is also the order areas are processed in.
const (
	AreaStopWords    = "stop-words"
	AreaReplacements = "replacements"
	AreaVocabulary   = "vocabulary"
	AreaSamples      = "samples"
)

// AreaNames lists every area voxi config import understands, in processing
// order.
var AreaNames = []string{AreaStopWords, AreaReplacements, AreaVocabulary, AreaSamples}

// vocabularyFileName mirrors the private literal in
// internal/speechcontext/context.go's VocabularyPath -- that package exposes
// no exported constant, and VocabularyPath itself can't be reused for a
// source directory (it joins home/.config/voxi/..., but sourceDir here IS
// already the .config/voxi-shaped directory, not its parent).
const vocabularyFileName = "vocabulary.txt"

// ImportOptions controls `voxi config import`.
type ImportOptions struct {
	// Overwrite replaces a colliding replacement entry (same From, case-
	// insensitive) instead of skipping it. Stop-words, vocabulary, and
	// samples are plain sets/collections with no separate overwrite
	// semantics -- a collision there always just means "already present" --
	// so this only changes replacement behavior, but every area's merge
	// function accepts it for a consistent call shape.
	Overwrite bool
	// Only restricts import to these areas (see AreaNames). Empty means
	// import every area found in sourceDir.
	Only []string
}

// ImportAreaSummary tallies one area's merge outcome, mirroring
// devsample.ImportSummary's field names for a consistent report shape
// across `sample import` and `config import`.
type ImportAreaSummary struct {
	Imported int
	Skipped  int // already present locally (or, for replacements, a collision left alone because --overwrite was not set)
	Failed   int // per-entry validation failure that did not abort the rest of the area (vocabulary.txt only)
}

// Import merges stop-words, replacements, vocabulary, and dev samples from
// sourceDir -- a local, previously-copied ~/.config/voxi-shaped directory,
// the same way sample import's sourceDir mirrors SamplesDir's layout --
// into home's local Voxi state. It never performs any transfer of its own.
//
// Each area is independent: a missing file/subdir for an area is not an
// error (nothing to import there), and a malformed stop-words.json or
// replacements.json is rejected whole rather than partially merged, so one
// bad file can't corrupt local state or silently drop entries. Area
// failures are reported to out and accumulated; Import returns a non-nil
// error if any area failed, but only after every requested area has been
// attempted, so a bad stop-words.json does not prevent replacements,
// vocabulary, or samples from importing.
func Import(home, sourceDir string, opts ImportOptions, maxVocabularyTermChars int, out io.Writer) error {
	if info, err := os.Stat(sourceDir); err != nil {
		return fmt.Errorf("source directory: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("source path %s is not a directory", sourceDir)
	}

	only := map[string]bool{}
	for _, a := range opts.Only {
		valid := false
		for _, name := range AreaNames {
			if a == name {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unknown --only area %q (want one of: %s)", a, strings.Join(AreaNames, ", "))
		}
		only[a] = true
	}
	want := func(area string) bool { return len(only) == 0 || only[area] }

	var failedAreas []string

	if want(AreaStopWords) {
		summary, found, err := importStopWords(home, sourceDir, opts.Overwrite)
		reportArea(out, AreaStopWords, "stop-words.json", summary, found, err, &failedAreas)
	}
	if want(AreaReplacements) {
		summary, found, err := importReplacements(home, sourceDir, opts.Overwrite)
		reportArea(out, AreaReplacements, "replacements.json", summary, found, err, &failedAreas)
	}
	if want(AreaVocabulary) {
		summary, found, err := importVocabulary(home, sourceDir, maxVocabularyTermChars)
		reportArea(out, AreaVocabulary, "vocabulary.txt", summary, found, err, &failedAreas)
	}
	if want(AreaSamples) {
		srcDir := filepath.Join(sourceDir, "samples")
		info, statErr := os.Stat(srcDir)
		if statErr != nil || !info.IsDir() {
			fmt.Fprintf(out, "%s: no samples/ found in %s, skipping\n", AreaSamples, sourceDir)
		} else if _, err := devsample.Import(home, srcDir, opts.Overwrite, out); err != nil {
			fmt.Fprintf(out, "%s: import failed: %v\n", AreaSamples, err)
			failedAreas = append(failedAreas, AreaSamples)
		}
	}

	if len(failedAreas) > 0 {
		return fmt.Errorf("config import failed for: %s (see output above)", strings.Join(failedAreas, ", "))
	}
	return nil
}

// reportArea prints one area's outcome in a consistent one-line format and
// records it as failed on error.
func reportArea(out io.Writer, area, fileName string, summary ImportAreaSummary, found bool, err error, failedAreas *[]string) {
	if err != nil {
		fmt.Fprintf(out, "%s: import failed: %v\n", area, err)
		*failedAreas = append(*failedAreas, area)
		return
	}
	if !found {
		fmt.Fprintf(out, "%s: no %s found in source, skipping\n", area, fileName)
		return
	}
	fmt.Fprintf(out, "%s: %d imported, %d skipped, %d failed\n", area, summary.Imported, summary.Skipped, summary.Failed)
}

func importStopWords(home, sourceDir string, overwrite bool) (ImportAreaSummary, bool, error) {
	_ = overwrite // union merge: no overwrite semantics for a plain set
	srcPath := filepath.Join(sourceDir, fileName)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return ImportAreaSummary{}, false, nil
	}
	src, err := Load(srcPath)
	if err != nil {
		return ImportAreaSummary{}, true, err
	}
	destPath := Path(home)
	dest, err := Load(destPath)
	if err != nil {
		return ImportAreaSummary{}, true, err
	}

	var summary ImportAreaSummary
	var imported, skipped int
	merged := dest
	merged.User, imported, skipped = mergeStringList(dest.User, src.User, true)
	summary.Imported += imported
	summary.Skipped += skipped
	merged.Disabled, imported, skipped = mergeStringList(dest.Disabled, src.Disabled, false)
	summary.Imported += imported
	summary.Skipped += skipped
	merged.SilenceArtifacts, imported, skipped = mergeStringList(dest.SilenceArtifacts, src.SilenceArtifacts, true)
	summary.Imported += imported
	summary.Skipped += skipped

	if summary.Imported > 0 {
		if err := Save(destPath, merged); err != nil {
			return summary, true, err
		}
	}
	return summary, true, nil
}

// mergeStringList unions src into dest, skipping entries already present
// (compared case-insensitively when fold is set, matching the equality
// semantics feedback.go's own normalize()/uniqueSorted() already apply to
// each field).
func mergeStringList(dest, src []string, fold bool) (merged []string, imported, skipped int) {
	seen := map[string]bool{}
	key := func(s string) string {
		if fold {
			return strings.ToLower(s)
		}
		return s
	}
	for _, d := range dest {
		seen[key(d)] = true
	}
	merged = append([]string(nil), dest...)
	for _, s := range src {
		k := key(s)
		if seen[k] {
			skipped++
			continue
		}
		seen[k] = true
		merged = append(merged, s)
		imported++
	}
	return merged, imported, skipped
}

func importReplacements(home, sourceDir string, overwrite bool) (ImportAreaSummary, bool, error) {
	srcPath := filepath.Join(sourceDir, replacementFileName)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return ImportAreaSummary{}, false, nil
	}
	src, err := LoadReplacements(srcPath)
	if err != nil {
		return ImportAreaSummary{}, true, err
	}
	destPath := ReplacementPath(home)
	dest, err := LoadReplacements(destPath)
	if err != nil {
		return ImportAreaSummary{}, true, err
	}

	merged, summary := mergeReplacements(dest, src, overwrite)
	if summary.Imported > 0 {
		if err := SaveReplacements(destPath, merged); err != nil {
			return summary, true, err
		}
	}
	return summary, true, nil
}

// mergeReplacements unions src into dest, keyed by From (case-insensitive,
// matching AddReplacement/RemoveReplacement's existing matching semantics).
// A colliding From is skipped by default; with overwrite set, the source
// entry replaces the local one.
func mergeReplacements(dest, src []Replacement, overwrite bool) ([]Replacement, ImportAreaSummary) {
	var summary ImportAreaSummary
	index := map[string]int{}
	merged := append([]Replacement(nil), dest...)
	for i, r := range merged {
		index[strings.ToLower(r.From)] = i
	}
	for _, r := range src {
		k := strings.ToLower(r.From)
		if i, exists := index[k]; exists {
			if !overwrite {
				summary.Skipped++
				continue
			}
			merged[i] = r
			summary.Imported++
			continue
		}
		index[k] = len(merged)
		merged = append(merged, r)
		summary.Imported++
	}
	return normalizeReplacements(merged), summary
}

func importVocabulary(home, sourceDir string, maxTermChars int) (ImportAreaSummary, bool, error) {
	srcPath := filepath.Join(sourceDir, vocabularyFileName)
	data, err := os.ReadFile(srcPath)
	if os.IsNotExist(err) {
		return ImportAreaSummary{}, false, nil
	}
	if err != nil {
		return ImportAreaSummary{}, true, fmt.Errorf("read source vocabulary: %w", err)
	}
	destPath := speechcontext.VocabularyPath(home)
	destTerms, err := speechcontext.LoadVocabulary(destPath, maxTermChars)
	if err != nil {
		return ImportAreaSummary{}, true, err
	}

	merged, summary := mergeVocabulary(destTerms, data, maxTermChars)
	if summary.Imported > 0 {
		if err := speechcontext.SaveVocabulary(destPath, merged, maxTermChars); err != nil {
			return summary, true, err
		}
	}
	return summary, true, nil
}

// mergeVocabulary unions the terms parsed from sourceData into destTerms.
// Unlike stop-words/replacements, a malformed line does not reject the
// whole file -- vocabulary.txt is plain line-delimited text with no
// structure to corrupt -- it is simply skipped and counted as Failed,
// applying the exact same per-term validation speechcontext.AddVocabulary
// already uses (NormalizeTerm).
func mergeVocabulary(destTerms []string, sourceData []byte, maxTermChars int) ([]string, ImportAreaSummary) {
	var summary ImportAreaSummary
	seen := map[string]bool{}
	for _, t := range destTerms {
		seen[strings.ToLower(t)] = true
	}
	merged := append([]string(nil), destTerms...)
	for _, raw := range speechcontext.ParseVocabulary(sourceData) {
		term, err := speechcontext.NormalizeTerm(raw, maxTermChars)
		if err != nil {
			summary.Failed++
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			summary.Skipped++
			continue
		}
		seen[key] = true
		merged = append(merged, term)
		summary.Imported++
	}
	return merged, summary
}
