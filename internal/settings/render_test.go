package settings

import (
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/config"
)

func TestRenderMenu(t *testing.T) {
	s := config.DefaultUserSettings()
	m := NewMenuModel(s, nil)

	rendered := RenderMenu(m, 72)
	if !strings.Contains(rendered, "VOXI SETTINGS") {
		t.Errorf("rendered menu missing header title")
	}
	if !strings.Contains(rendered, "LLM Post-Processing Cleaner") {
		t.Errorf("rendered menu missing LLM Cleaner item")
	}
	if !strings.Contains(rendered, "Save & Apply Changes") {
		t.Errorf("rendered menu missing Save action")
	}
	if !strings.Contains(rendered, "❯ ") {
		t.Errorf("rendered menu missing cursor marker")
	}
}

func TestRenderSummary(t *testing.T) {
	s := &config.UserSettings{
		LLMCleaner:       true,
		CleanupModel:     "qwen3-4b-instruct-2507-q4",
		ASRModel:         "cohere-transcribe-03-2026",
		TypeDelayMs:      5,
		DictationHistory: true,
		ModifierGating:   false,
	}

	summary := RenderSummary(s, "/home/testuser")
	if !strings.Contains(summary, "Voxi Configuration Summary:") {
		t.Errorf("summary missing title")
	}
	if !strings.Contains(summary, "LLM Cleaner:           [✓] Enabled") {
		t.Errorf("summary missing enabled LLM Cleaner")
	}
	if !strings.Contains(summary, "Typing Delay:          5ms") {
		t.Errorf("summary missing typing delay")
	}
	if !strings.Contains(summary, "Modifier Gating:       [ ] Disabled") {
		t.Errorf("summary missing disabled modifier gating")
	}
	if !strings.Contains(summary, "/home/testuser/.config/voxi/config.yaml") {
		t.Errorf("summary missing yaml path")
	}
}

func TestRenderJSON(t *testing.T) {
	s := &config.UserSettings{
		LLMCleaner:       true,
		CleanupModel:     "smollm3-3b-instruct-q4",
		OpenAIBaseURL:    "http://127.0.0.1:8734/v1",
		ASRModel:         "cohere-transcribe-03-2026",
		TypeDelayMs:      1,
		DictationHistory: true,
		ModifierGating:   true,
	}

	out, err := RenderJSON(s)
	if err != nil {
		t.Fatalf("RenderJSON failed: %v", err)
	}
	if !strings.Contains(out, `"llm_cleaner": true`) {
		t.Errorf("json missing llm_cleaner: %s", out)
	}
	if !strings.Contains(out, `"cleanup_model": "smollm3-3b-instruct-q4"`) {
		t.Errorf("json missing cleanup_model: %s", out)
	}
}
