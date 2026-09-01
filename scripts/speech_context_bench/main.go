// Command speech_context_bench compares prompted and unprompted small.en on a
// private local WAV corpus described by a committed text-only manifest.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"ubunatic.com/voxi/internal/asr"
	"ubunatic.com/voxi/internal/speechcontext"
	"ubunatic.com/voxi/spec"
)

type fixture struct {
	ID       string
	File     string
	Expected string
	Keyterms []string
}

type result struct {
	ID              string  `json:"id"`
	Prompted        bool    `json:"prompted"`
	Transcript      string  `json:"transcript"`
	WER             float64 `json:"wer"`
	KeytermsFound   int     `json:"keyterms_found"`
	KeytermsTotal   int     `json:"keyterms_total"`
	LatencyMS       int64   `json:"latency_ms"`
	AdjacentRepeats int     `json:"adjacent_repeats"`
}

type report struct {
	Model   string    `json:"model"`
	Prompt  string    `json:"prompt"`
	Runs    []result  `json:"runs"`
	Summary []summary `json:"summary"`
}

type summary struct {
	Prompted        bool    `json:"prompted"`
	MeanWER         float64 `json:"mean_wer"`
	KeytermRecall   float64 `json:"keyterm_recall"`
	MedianLatencyMS int64   `json:"median_latency_ms"`
	AdjacentRepeats int     `json:"adjacent_repeats"`
}

func main() {
	corpus := flag.String("corpus", "testdata/speech-context", "directory containing corpus.tsv and private WAV files")
	model := flag.String("model", "small.en", "local Whisper model (benchmark target is small.en)")
	threads := flag.Int("threads", 6, "voxtype inference threads")
	flag.Parse()
	if err := run(context.Background(), *corpus, *model, *threads); err != nil {
		fmt.Fprintln(os.Stderr, "speech-context benchmark:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, corpus, model string, threads int) error {
	if model != "small.en" {
		return fmt.Errorf("model must be small.en, got %q", model)
	}
	fixtures, err := loadManifest(filepath.Join(corpus, "corpus.tsv"))
	if err != nil {
		return err
	}
	modelSpec, err := spec.LoadModels()
	if err != nil {
		return err
	}
	var explicit []string
	for _, item := range fixtures {
		explicit = append(explicit, item.Keyterms...)
	}
	prompt := speechcontext.Build(speechcontext.Options{
		Enabled:      true,
		PromptPrefix: modelSpec.SpeechContext.PromptPrefix,
		MaxTerms:     modelSpec.SpeechContext.MaxTerms,
		MaxChars:     modelSpec.SpeechContext.MaxChars,
		MaxTermChars: modelSpec.SpeechContext.MaxTermChars,
	}, speechcontext.Sources{Explicit: explicit, Static: modelSpec.SpeechContext.Terms})

	report := report{Model: model, Prompt: prompt}
	for _, item := range fixtures {
		wavPath := filepath.Join(corpus, item.File)
		if _, statErr := os.Stat(wavPath); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("missing private fixture %s; see %s", wavPath, filepath.Join(corpus, "README.md"))
			}
			return statErr
		}
		for _, prompted := range []bool{false, true} {
			initialPrompt := ""
			if prompted {
				initialPrompt = prompt
			}
			transcript, elapsed, transcribeErr := transcribe(ctx, model, threads, wavPath, initialPrompt)
			if transcribeErr != nil {
				return fmt.Errorf("%s prompted=%t: %w", item.ID, prompted, transcribeErr)
			}
			found := 0
			for _, term := range item.Keyterms {
				if containsTerm(transcript, term) {
					found++
				}
			}
			report.Runs = append(report.Runs, result{
				ID: item.ID, Prompted: prompted, Transcript: transcript,
				WER: wordErrorRate(item.Expected, transcript), KeytermsFound: found,
				KeytermsTotal: len(item.Keyterms), LatencyMS: elapsed.Milliseconds(),
				AdjacentRepeats: adjacentRepeats(transcript),
			})
		}
	}
	report.Summary = []summary{summarize(report.Runs, false), summarize(report.Runs, true)}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func loadManifest(path string) ([]fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var fixtures []fixture
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Split(text, "\t")
		if len(fields) != 4 {
			return nil, fmt.Errorf("%s:%d: want four tab-separated fields", path, line)
		}
		var keyterms []string
		if fields[3] != "" {
			keyterms = strings.Split(fields[3], "|")
		}
		fixtures = append(fixtures, fixture{ID: fields[0], File: fields[1], Expected: fields[2], Keyterms: keyterms})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("%s has no fixtures", path)
	}
	return fixtures, nil
}

func transcribe(ctx context.Context, model string, threads int, wavPath, prompt string) (string, time.Duration, error) {
	args := []string{"--model", model, "--threads", fmt.Sprint(threads)}
	if prompt != "" {
		args = append(args, "--initial-prompt", prompt)
	}
	args = append(args, "-q", "transcribe", wavPath)
	cmd := exec.CommandContext(ctx, "voxtype", args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "RUST_LOG=error")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	started := time.Now()
	err := cmd.Run()
	return asr.CleanWhisperTranscript(stdout.String(), nil), time.Since(started), err
}

func words(text string) []string {
	raw := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._+#@-", r)
	})
	words := make([]string, 0, len(raw))
	for _, word := range raw {
		if word = strings.Trim(word, "._-"); word != "" {
			words = append(words, word)
		}
	}
	return words
}

func wordErrorRate(expected, actual string) float64 {
	want, got := words(expected), words(actual)
	if len(want) == 0 {
		if len(got) == 0 {
			return 0
		}
		return 1
	}
	previous := make([]int, len(got)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, wantWord := range want {
		current := make([]int, len(got)+1)
		current[0] = i + 1
		for j, gotWord := range got {
			cost := 1
			if wantWord == gotWord {
				cost = 0
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return float64(previous[len(got)]) / float64(len(want))
}

func containsTerm(transcript, term string) bool {
	return strings.Contains(" "+strings.Join(words(transcript), " ")+" ", " "+strings.Join(words(term), " ")+" ")
}

func adjacentRepeats(text string) int {
	items := words(text)
	count := 0
	for i := 1; i < len(items); i++ {
		if items[i] == items[i-1] {
			count++
		}
	}
	return count
}

func median(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	if len(values)%2 == 0 {
		middle := len(values) / 2
		return (values[middle-1] + values[middle]) / 2
	}
	return values[len(values)/2]
}

func summarize(results []result, prompted bool) summary {
	answer := summary{Prompted: prompted}
	var latencies []int64
	var werTotal float64
	found, total := 0, 0
	count := 0
	for _, item := range results {
		if item.Prompted != prompted {
			continue
		}
		count++
		werTotal += item.WER
		found += item.KeytermsFound
		total += item.KeytermsTotal
		answer.AdjacentRepeats += item.AdjacentRepeats
		latencies = append(latencies, item.LatencyMS)
	}
	if count > 0 {
		answer.MeanWER = werTotal / float64(count)
	}
	if total > 0 {
		answer.KeytermRecall = float64(found) / float64(total)
	}
	answer.MedianLatencyMS = median(latencies)
	return answer
}
