package sample

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MigrationPlan describes the legacy samples that can safely enter a store.
type MigrationPlan struct {
	Items    []MigrationItem
	Problems []string
}

// MigrationItem is one planned legacy sample copy.
type MigrationItem struct {
	Sample Sample
	Reason string
}

// PlanMigration reads a legacy sample directory without modifying either it or
// the destination store.
func PlanMigration(dir string, now time.Time) (MigrationPlan, error) {
	samples, err := LoadLegacyTSV(dir)
	if err != nil {
		return MigrationPlan{}, fmt.Errorf("read legacy corpus: %w", err)
	}
	allowed, err := legacyAllowlist(filepath.Join(dir, "voice-training.txt"))
	if err != nil {
		return MigrationPlan{}, err
	}
	byID := make(map[string]Sample, len(samples))
	for _, x := range samples {
		byID[x.ID] = x
	}
	plan := MigrationPlan{}
	for id := range allowed {
		if _, ok := byID[id]; !ok {
			plan.Problems = append(plan.Problems, fmt.Sprintf("%s: allowlisted but has no corpus.tsv row", id))
		}
	}
	for _, x := range samples {
		if _, err := os.Stat(filepath.Join(dir, x.Audio)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				plan.Problems = append(plan.Problems, fmt.Sprintf("%s: WAV %s is missing", x.ID, x.Audio))
				continue
			}
			return MigrationPlan{}, fmt.Errorf("stat legacy WAV for %s: %w", x.ID, err)
		}
		item := MigrationItem{Sample: x}
		item.Sample.Created = now.UTC()
		switch {
		case allowed[x.ID]:
			item.Sample.Purpose = Voice
			item.Sample.Consent = &item.Sample.Created
			item.Sample.Source = "migration:voice-training.txt"
			item.Reason = "allowlisted in voice-training.txt; consent recorded"
		case isNoiseTranscript(x.Transcript):
			item.Sample.Purpose = Noise
			item.Sample.Source = "migration:corpus.tsv"
			item.Reason = "empty or bracketed noise transcript"
		default:
			item.Sample.Purpose = Dictation
			item.Sample.Source = "migration:corpus.tsv"
			item.Reason = "transcribed legacy sample"
		}
		plan.Items = append(plan.Items, item)
	}
	sort.Slice(plan.Items, func(i, j int) bool { return plan.Items[i].Sample.ID < plan.Items[j].Sample.ID })
	sort.Strings(plan.Problems)
	return plan, nil
}

func legacyAllowlist(path string) (map[string]bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read voice-training allowlist: %w", err)
	}
	allowed := make(map[string]bool)
	for _, line := range strings.Split(string(b), "\n") {
		id := strings.TrimSpace(line)
		if id == "" || strings.HasPrefix(id, "#") {
			continue
		}
		if !validID(id) {
			return nil, fmt.Errorf("voice-training.txt: invalid id %q", id)
		}
		allowed[id] = true
	}
	return allowed, nil
}

func isNoiseTranscript(text string) bool {
	text = strings.TrimSpace(text)
	return text == "" || strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]")
}

// Migrate copies every planned WAV into store and verifies its size and SHA-256.
func Migrate(store *Store, dir string, plan MigrationPlan) error {
	if len(plan.Items) == 0 {
		return errors.New("migration has no valid samples")
	}
	for _, item := range plan.Items {
		source := filepath.Join(dir, item.Sample.Audio)
		if existing, err := store.Get(item.Sample.ID); err == nil {
			if err := sameFile(source, store.AudioPath(existing)); err != nil {
				return fmt.Errorf("hash conflict for %s: %w", item.Sample.ID, err)
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, item := range plan.Items {
		if exists, err := store.Has(item.Sample.ID); err != nil || exists {
			if err != nil {
				return err
			}
			continue
		}
		if err := store.Put(item.Sample, filepath.Join(dir, item.Sample.Audio)); err != nil {
			return fmt.Errorf("migrate %s: %w", item.Sample.ID, err)
		}
		if err := sameFile(filepath.Join(dir, item.Sample.Audio), store.AudioPath(item.Sample)); err != nil {
			return fmt.Errorf("verify %s: %w", item.Sample.ID, err)
		}
	}
	return nil
}

func sameFile(left, right string) error {
	a, err := os.Open(left)
	if err != nil {
		return err
	}
	defer a.Close()
	b, err := os.Open(right)
	if err != nil {
		return err
	}
	defer b.Close()
	ai, err := a.Stat()
	if err != nil {
		return err
	}
	bi, err := b.Stat()
	if err != nil {
		return err
	}
	if ai.Size() != bi.Size() {
		return fmt.Errorf("size differs (%d != %d)", ai.Size(), bi.Size())
	}
	ah := sha256.New()
	bh := sha256.New()
	if _, err = io.Copy(ah, a); err != nil {
		return err
	}
	if _, err = io.Copy(bh, b); err != nil {
		return err
	}
	if string(ah.Sum(nil)) != string(bh.Sum(nil)) {
		return errors.New("SHA-256 differs")
	}
	return nil
}
