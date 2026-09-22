package monitor

import (
	"bytes"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/asr"
)

// TestDaemonsBoxASRLineOwnLine verifies the ASR backend state renders as its
// own line in the "[d] active daemons & health" box, first among the box's
// lines, so it is never truncated away by an overflowing process list --
// issue 136 M3 (placement bug in M2's suffix-append approach).
func TestDaemonsBoxASRLineOwnLine(t *testing.T) {
	cases := []struct {
		name  string
		state ASRBackendState
		want  string
	}{
		{
			name:  "online",
			state: ASRBackendState{Applicable: true, Online: true, Engine: "openai-transcribe", Endpoint: "http://127.0.0.1:18131"},
			want:  "online",
		},
		{
			name:  "offline",
			state: ASRBackendState{Applicable: true, Online: false, Engine: "openai-transcribe", Endpoint: "http://127.0.0.1:18131"},
			want:  "OFFLINE",
		},
		{
			name:  "no-server",
			state: ASRBackendState{Applicable: false, Engine: "whisper", Binary: "voxtype"},
			want:  "no server",
		},
	}

	// Several processes, long enough that the old single-line join would
	// truncate the appended ASR suffix away at a realistic ~90-column width.
	procs := []ProcessResource{
		{PID: 1234, Name: "voxi-modifierd", RSSBytes: 12 * 1024 * 1024},
		{PID: 5678, Name: "dotoold", RSSBytes: 8 * 1024 * 1024},
		{PID: 9012, Name: "harnez", RSSBytes: 20 * 1024 * 1024},
		{PID: 3456, Name: "voxi-agent", RSSBytes: 40 * 1024 * 1024},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := VoiceResourceReport{
				Processes:  procs,
				ASRBackend: tc.state,
			}
			var buf bytes.Buffer
			PrintVoiceResourceReport(&buf, report, ResourceSections{Daemons: true})
			out := asr.StripANSI(buf.String())

			var asrLine string
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "asr:") {
					asrLine = line
					break
				}
			}
			if asrLine == "" {
				t.Fatalf("expected an ASR backend line in daemons box output, got:\n%s", out)
			}
			if !strings.Contains(asrLine, tc.want) {
				t.Errorf("ASR line %q does not contain expected marker %q", asrLine, tc.want)
			}
			// Must be its own line, not a suffix sharing the (long, often
			// truncated) process-summary line.
			if strings.Contains(asrLine, "voxi-modifierd") {
				t.Errorf("ASR line unexpectedly shares a line with the process list: %q", asrLine)
			}
			// The ASR line must appear before the process-summary line so it
			// survives truncation of the daemons box at normal terminal
			// width (issue 136 M3).
			procLineIdx := strings.Index(out, "voxi-modifierd")
			asrLineIdx := strings.Index(out, "asr:")
			if asrLineIdx == -1 || procLineIdx == -1 || asrLineIdx > procLineIdx {
				t.Errorf("expected ASR line before process list; asr idx=%d proc idx=%d", asrLineIdx, procLineIdx)
			}
			// Sanity: fits comfortably within a ~90-column terminal.
			if w := StringDisplayWidth(asrLine); w > 90 {
				t.Errorf("ASR line too wide for a 90-column terminal: %d", w)
			}
		})
	}
}
