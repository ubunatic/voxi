// Command speech_context_bench compares prompted and unprompted small.en on a
// private local sample store.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode"

	"ubunatic.com/voxi/internal/asr"
	"ubunatic.com/voxi/internal/sample"
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
	corpus := flag.String("corpus", sample.Root(os.Getenv("XDG_DATA_HOME")), "sample store root")
	model := flag.String("model", "small.en", "local Whisper model (benchmark target is small.en)")
	threads := flag.Int("threads", 6, "voxtype inference threads")
	listOnly := flag.Bool("list-only", false, "list matching store samples without running ASR")
	flag.Parse()
	if err := run(context.Background(), *corpus, *model, *threads, *listOnly); err != nil {
		fmt.Fprintln(os.Stderr, "speech-context benchmark:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, corpus, model string, threads int, listOnly bool) error {
	if model != "small.en" {
		return fmt.Errorf("model must be small.en, got %q", model)
	}
	store, err := sample.OpenReadOnly(corpus)
	if err != nil {
		return err
	}
	items, err := store.List()
	if err != nil {
		return err
	}
	fixtures := make([]fixture, 0, len(items))
	for _, item := range items {
		fixtures = append(fixtures, fixture{
			ID: item.ID, File: store.AudioPath(item),
			Expected: item.Transcript, Keyterms: item.Keyterms,
		})
	}
	if len(fixtures) == 0 {
		return fmt.Errorf("%s has no samples", corpus)
	}
	if listOnly {
		fmt.Printf("found %d sample(s) in %s\n", len(fixtures), corpus)
		return nil
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
		wavPath := item.File
		if _, statErr := os.Stat(wavPath); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("missing sample audio %s", wavPath)
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
