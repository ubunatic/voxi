package monitor

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseSections(t *testing.T) {
	s1 := ParseSections("all")
	if !s1.Speed || !s1.Hardware || !s1.Transcript || !s1.Daemons {
		t.Fatalf("expected all true for 'all', got %+v", s1)
	}

	s2 := ParseSections("s,t")
	if !s2.Speed || !s2.Transcript || s2.Hardware || s2.Daemons {
		t.Fatalf("expected only speed and transcript true, got %+v", s2)
	}
}

func TestDisplayWidthAndTruncate(t *testing.T) {
	s := "Hello \x1b[32mWorld\x1b[0m 🚀"
	visLen := StringDisplayWidth(s)
	// "Hello " (6) + "World" (5) + " " (1) + "🚀" (2) = 14
	if visLen != 14 {
		t.Fatalf("expected visual length 14, got %d", visLen)
	}

	trunc := TruncateLineANSI(s, 10)
	if StringDisplayWidth(trunc) > 10 {
		t.Fatalf("truncated string exceeds width 10: %q (len %d)", trunc, StringDisplayWidth(trunc))
	}
}

func TestRenderBoxLines(t *testing.T) {
	box := BoxSpec{
		Title: "Test Box",
		Lines: []string{"Line 1", "Line 2 is longer"},
		Width: 30,
	}

	lines := RenderBoxLines(box)
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (top, 2 body, bot), got %d", len(lines))
	}

	for _, l := range lines {
		if StringDisplayWidth(l) != 30 {
			t.Fatalf("line width != 30: %q (len %d)", l, StringDisplayWidth(l))
		}
	}
}

func TestPrintVoiceResourceReport(t *testing.T) {
	var out bytes.Buffer
	report := VoiceResourceReport{
		Mode:          "eager",
		RecordStatus:  "idle",
		ActiveService: "voxi-eager.service",
		GPUAccel:      "AMD Radeon Vulkan 1.4",
		ActiveModel:   "small.en",
	}

	PrintVoiceResourceReport(&out, report, DefaultResourceSections())
	output := out.String()

	if !strings.Contains(output, "voice & speed") || !strings.Contains(output, "hardware load") {
		t.Fatalf("missing expected sections in report: %s", output)
	}
}
