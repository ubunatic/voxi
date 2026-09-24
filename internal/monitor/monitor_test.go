package monitor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

func TestParseSections(t *testing.T) {
	s1 := ParseSections("all")
	if !s1.Speed || !s1.Hardware || !s1.Transcript || !s1.Daemons || !s1.TTS {
		t.Fatalf("expected all sections for 'all', got %+v", s1)
	}

	s2 := ParseSections("s,t")
	if !s2.Speed || !s2.Transcript || s2.Hardware || s2.Daemons {
		t.Fatalf("expected only speed and transcript true, got %+v", s2)
	}
}

func TestDefaultResourceSectionsFollowActionSpec(t *testing.T) {
	got := DefaultResourceSections()
	if !got.Speed || got.Hardware || !got.Transcript || !got.Daemons || !got.TTS {
		t.Fatalf("unexpected default sections: %+v", got)
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

	sections := DefaultResourceSections()
	sections.Hardware = true
	PrintVoiceResourceReport(&out, report, sections)
	output := out.String()

	if !strings.Contains(output, "voice & speed") || !strings.Contains(output, "hardware load") {
		t.Fatalf("missing expected sections in report: %s", output)
	}
}

func TestDetectActiveModel(t *testing.T) {
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return "/tmp/nonexistent-home"
			}
			return ""
		},
		Run: func(_ context.Context, _ string, _ ...string) error {
			return errors.New("service inactive")
		},
	}
	model := detectActiveModel(d)
	if model != "cohere-transcribe-03-2026" {
		t.Fatalf("expected cohere-transcribe-03-2026 default, got %q", model)
	}
}

// TestDetectActiveModel_UserSettingsWin is the issue 135 regression: voxi
// user settings name a non-default model and voxtype.service is inactive,
// so the monitor must report that model, not the spec default.
func TestDetectActiveModel_UserSettingsWin(t *testing.T) {
	home := t.TempDir()
	voxiDir := filepath.Join(home, ".config", "voxi")
	if err := os.MkdirAll(voxiDir, 0o755); err != nil {
		t.Fatalf("mkdir voxi config dir: %v", err)
	}
	yaml := "asr_model: r2t2-confucius4\n"
	if err := os.WriteFile(filepath.Join(voxiDir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			return ""
		},
		Run: func(_ context.Context, _ string, _ ...string) error {
			return errors.New("service inactive")
		},
	}
	model := detectActiveModel(d)
	if model != "r2t2-confucius4" {
		t.Fatalf("expected r2t2-confucius4 from user settings, got %q", model)
	}
}

// TestDetectActiveModel_LegacyVoxtypeFallback covers the case where voxi's
// own user settings cannot be resolved (e.g. a broken config.yaml) but
// voxtype.service is the real active backend: the legacy config.toml model
// must still win.
func TestDetectActiveModel_LegacyVoxtypeFallback(t *testing.T) {
	home := t.TempDir()
	voxiDir := filepath.Join(home, ".config", "voxi")
	if err := os.MkdirAll(voxiDir, 0o755); err != nil {
		t.Fatalf("mkdir voxi config dir: %v", err)
	}
	// Malformed YAML makes config.LoadUserSettings return an error, so
	// detectActiveModel must fall through to the legacy voxtype path.
	if err := os.WriteFile(filepath.Join(voxiDir, "config.yaml"), []byte("asr_model: [unterminated\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	voxtypeDir := filepath.Join(home, ".config", "voxtype")
	if err := os.MkdirAll(voxtypeDir, 0o755); err != nil {
		t.Fatalf("mkdir voxtype config dir: %v", err)
	}
	toml := "model = \"whisper-large-v3\"\n"
	if err := os.WriteFile(filepath.Join(voxtypeDir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			return ""
		},
		Run: func(_ context.Context, _ string, _ ...string) error {
			return nil // voxtype.service active
		},
	}
	model := detectActiveModel(d)
	if model != "whisper-large-v3" {
		t.Fatalf("expected whisper-large-v3 from legacy voxtype config, got %q", model)
	}
}

// TestDetectActiveModel_SpecDefaultFallback covers the case where neither
// voxi user settings nor the legacy voxtype path yield anything useful and
// voxtype.service is inactive: the spec default must be reported.
func TestDetectActiveModel_SpecDefaultFallback(t *testing.T) {
	home := t.TempDir()

	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			return ""
		},
		Run: func(_ context.Context, _ string, _ ...string) error {
			return errors.New("service inactive")
		},
	}
	model := detectActiveModel(d)
	if model != "cohere-transcribe-03-2026" {
		t.Fatalf("expected cohere-transcribe-03-2026 default, got %q", model)
	}
}
