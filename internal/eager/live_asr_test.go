package eager

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/spec"
)

// TestLiveASR_R2T2Corpus drives the REAL engine path (transcribeOpenAIWAV,
// the same function runEagerCaptureSessionAt's runTranscribe closure calls
// for engine == openai-transcribe) against a genuinely running llama-server
// serving the r2t2-confucius4 spec entry, using the private local sample
// corpus at ~/.config/voxi/samples/corpus.tsv (issue 134).
//
// This test never starts llama-server itself (see issue 134's Canary-first
// rule: don't build/download/launch the external dependency from inside a
// test) and must not fail `go test ./...` when no server is running, so it
// is gated on BOTH an explicit opt-in env var and a live reachability
// check:
//
//	VOXI_LIVE_ASR=1 go test ./internal/eager/ -run TestLiveASR_R2T2Corpus -v
//
// It intentionally does not assert on transcript wording -- R2T2 accuracy
// varies clip to clip and is a human judgment call, not a pass/fail gate.
// It only asserts what's safe to assert unconditionally: a non-empty
// transcript, and that the "<asr_text>" envelope marker (issue 134 §6,
// strip_before_marker) never leaks into the final text. Per-clip wall time
// and a simple word-level similarity/WER against corpus.tsv's expected
// transcript are printed in a table for a human to read.
func TestLiveASR_R2T2Corpus(t *testing.T) {
	if os.Getenv("VOXI_LIVE_ASR") != "1" {
		t.Skip("VOXI_LIVE_ASR not set to 1; skipping live ASR corpus test (see test doc comment to run it)")
	}

	modelSpec, err := spec.LoadModels()
	if err != nil {
		t.Fatalf("spec.LoadModels: %v", err)
	}
	m, ok := modelSpec.Models["r2t2-confucius4"]
	if !ok {
		t.Fatal("models.yaml missing r2t2-confucius4")
	}
	baseURL := resolveOpenAIASRBaseURL(m.BaseURL, "")
	if baseURL == "" {
		t.Fatal("r2t2-confucius4 has no base_url in spec/models.yaml")
	}

	if !isReachable(baseURL, 500*time.Millisecond) {
		t.Skipf("llama-server not reachable at %s; skipping live ASR corpus test", baseURL)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	corpusDir := filepath.Join(home, ".config", "voxi", "samples")
	clips, err := loadCorpus(filepath.Join(corpusDir, "corpus.tsv"))
	if err != nil {
		t.Fatalf("loadCorpus: %v", err)
	}

	type result struct {
		id         string
		wallTime   time.Duration
		similarity float64
		got        string
		want       string
		err        error
	}
	var results []result
	usable := 0
	for _, clip := range clips {
		if clip.expected == "" {
			continue // no expected transcript to compare against -- skip (per issue 134 instructions)
		}
		usable++
		wavPath := filepath.Join(corpusDir, clip.wavFile)
		if _, statErr := os.Stat(wavPath); statErr != nil {
			t.Errorf("clip %s: wav file missing: %v", clip.id, statErr)
			continue
		}

		start := time.Now()
		got, transcribeErr := transcribeOpenAIWAV(context.Background(), wavPath, baseURL, m.APIModel, m.ResponseFormat, m.StripBeforeMarker)
		elapsed := time.Since(start)

		r := result{id: clip.id, wallTime: elapsed, want: clip.expected, got: got, err: transcribeErr}
		if transcribeErr == nil {
			r.similarity = wordSimilarity(clip.expected, got)
		}
		results = append(results, r)

		if transcribeErr != nil {
			t.Errorf("clip %s: transcribeOpenAIWAV: %v", clip.id, transcribeErr)
			continue
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("clip %s: transcript is empty", clip.id)
		}
		if strings.Contains(got, "<asr_text>") {
			t.Errorf("clip %s: transcript leaks the <asr_text> envelope marker: %q", clip.id, got)
		}
	}

	if usable == 0 {
		t.Fatal("no usable corpus clips (all had empty expected transcripts)")
	}

	fmt.Printf("\n%-32s %10s %6s  %s\n", "clip", "wall", "sim", "transcript (want / got)")
	fmt.Println(strings.Repeat("-", 100))
	for _, r := range results {
		if r.err != nil {
			fmt.Printf("%-32s %10s %6s  ERROR: %v\n", r.id, r.wallTime.Round(time.Millisecond), "-", r.err)
			continue
		}
		fmt.Printf("%-32s %10s %5.0f%%  want=%q got=%q\n", r.id, r.wallTime.Round(time.Millisecond), r.similarity*100, r.want, r.got)
	}
}

// isReachable is a cheap, fast liveness probe: it only checks that
// something is listening on the endpoint's host:port, not that it speaks
// the ASR protocol correctly -- a real transcribeOpenAIWAV call later in
// the test is what actually exercises the protocol.
func isReachable(baseURL string, timeout time.Duration) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := u.Host
	if host == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// corpusClip is one row of corpus.tsv: tab-separated id, wav file,
// expected transcript, keyterms (pipe-separated, unused by this test).
type corpusClip struct {
	id       string
	wavFile  string
	expected string
}

// loadCorpus parses corpus.tsv, skipping blank lines and lines starting
// with "#" (comments and the "#ts <id> <timestamp>" recording markers seen
// in ~/.config/voxi/samples/corpus.tsv).
func loadCorpus(path string) ([]corpusClip, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var clips []corpusClip
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		clip := corpusClip{id: fields[0], wavFile: fields[1]}
		if len(fields) >= 3 {
			clip.expected = strings.TrimSpace(fields[2])
		}
		clips = append(clips, clip)
	}
	return clips, nil
}

// wordSimilarity returns a 0..1 score (1 - normalized word-level Levenshtein
// distance) between want and got, case-insensitive and punctuation-loose.
// This is a simple, dependency-free accuracy signal for the printed table
// -- not a pass/fail gate (per issue 134: don't hard-fail on wording).
func wordSimilarity(want, got string) float64 {
	a := normalizeWords(want)
	b := normalizeWords(got)
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	dist := wordLevenshtein(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 1
	}
	sim := 1 - float64(dist)/float64(maxLen)
	if sim < 0 {
		sim = 0
	}
	return sim
}

func normalizeWords(s string) []string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == ' ':
			return r
		default:
			return ' '
		}
	}, s)
	return strings.Fields(s)
}

// wordLevenshtein computes the classic edit distance over word tokens
// (substitution/insertion/deletion each cost 1) -- the standard basis for
// word error rate.
func wordLevenshtein(a, b []string) int {
	n, m := len(a), len(b)
	prev := make([]int, m+1)
	curr := make([]int, m+1)
	for j := 0; j <= m; j++ {
		prev[j] = j
	}
	for i := 1; i <= n; i++ {
		curr[0] = i
		for j := 1; j <= m; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			min := del
			if ins < min {
				min = ins
			}
			if sub < min {
				min = sub
			}
			curr[j] = min
		}
		prev, curr = curr, prev
	}
	return prev[m]
}
