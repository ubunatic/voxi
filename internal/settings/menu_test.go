package settings

import (
	"testing"

	"ubunatic.com/voxi/internal/config"
)

func TestMenuModelNavigation(t *testing.T) {
	s := config.DefaultUserSettings()
	m := NewMenuModel(s, nil)

	if len(m.Items) != 9 {
		t.Fatalf("expected 9 items, got %d", len(m.Items))
	}
	if m.Cursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", m.Cursor)
	}

	// Move Down
	m.MoveDown()
	if m.Cursor != 1 {
		t.Errorf("cursor after MoveDown = %d, want 1", m.Cursor)
	}

	// Move Up
	m.MoveUp()
	if m.Cursor != 0 {
		t.Errorf("cursor after MoveUp = %d, want 0", m.Cursor)
	}

	// Wrap Up
	m.MoveUp()
	if m.Cursor != len(m.Items)-1 {
		t.Errorf("cursor after wrap Up = %d, want %d", m.Cursor, len(m.Items)-1)
	}

	// Wrap Down
	m.MoveDown()
	if m.Cursor != 0 {
		t.Errorf("cursor after wrap Down = %d, want 0", m.Cursor)
	}
}

func TestMenuModelTogglesAndCycles(t *testing.T) {
	s := &config.UserSettings{
		LLMCleaner:       false,
		CleanupModel:     "qwen3-4b-instruct-2507-q4",
		ASRModel:         "cohere-transcribe-03-2026",
		TypeDelayMs:      0,
		DictationHistory: true,
		ModifierGating:   true,
	}
	m := NewMenuModel(s, []string{"cohere-transcribe-03-2026", "large-v3-turbo", "small.en", "base.en"})

	// 1. LLM Cleaner (Cursor 1): Toggle bool
	m.Cursor = 1
	if m.CurrentItem().BoolValue != false {
		t.Fatalf("initial LLMCleaner should be false")
	}
	m.ToggleOrNext()
	if m.CurrentItem().BoolValue != true {
		t.Errorf("LLMCleaner after toggle = %v, want true", m.CurrentItem().BoolValue)
	}
	if !m.Modified {
		t.Errorf("expected Modified=true after change")
	}

	// 2. Cleanup Model (Cursor 2): Cycle choice
	m.Cursor = 2
	initialModel := m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex]
	m.Next()
	newModel := m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex]
	if newModel == initialModel {
		t.Errorf("CleanupModel did not change on Next()")
	}
	m.Prev()
	revertedModel := m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex]
	if revertedModel != initialModel {
		t.Errorf("CleanupModel after Prev() = %s, want %s", revertedModel, initialModel)
	}

	// 3. Cleanup backend (Cursor 0): Cycle choice
	m.Cursor = 0
	initialBackend := m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex]
	m.Next()
	if m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex] == initialBackend {
		t.Errorf("CleanupBackend did not change on Next()")
	}

	// 4. ASR Model (Cursor 3): Cycle choice
	m.Cursor = 3
	initialASR := m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex]
	m.Next()
	newASR := m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex]
	if newASR == initialASR {
		t.Errorf("ASRModel did not change on Next()")
	}

	// 5. Typing Delay (Cursor 4): Cycle choice
	m.Cursor = 4
	if m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex] != "0ms" {
		t.Errorf("initial delay = %s, want 0ms", m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex])
	}
	m.Next()
	if m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex] != "1ms" {
		t.Errorf("next delay = %s, want 1ms", m.CurrentItem().Choices[m.CurrentItem().ChoiceIndex])
	}

	// 6. Save action (Cursor 7)
	m.Cursor = 7
	m.ToggleOrNext()
	if !m.Saved || !m.Closed {
		t.Errorf("expected Saved=true, Closed=true on Save row action")
	}

	// Convert to UserSettings
	updated := m.ToUserSettings(s)
	if updated.LLMCleaner != true {
		t.Errorf("updated LLMCleaner = %v, want true", updated.LLMCleaner)
	}
	if updated.TypeDelayMs != 1 {
		t.Errorf("updated TypeDelayMs = %d, want 1", updated.TypeDelayMs)
	}
}

func TestMenuModelKeyDispatch(t *testing.T) {
	s := config.DefaultUserSettings()
	m := NewMenuModel(s, nil)

	// Dispatch KeyDown
	m.HandleKey(KeyDown)
	if m.Cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.Cursor)
	}

	// Dispatch KeyUp
	m.HandleKey(KeyUp)
	if m.Cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.Cursor)
	}

	// Dispatch KeySave
	m.HandleKey(KeySave)
	if !m.Saved || !m.Closed {
		t.Errorf("KeySave did not save/close model")
	}

	// Fresh model -> KeyQuit
	m2 := NewMenuModel(s, nil)
	m2.HandleKey(KeyQuit)
	if m2.Saved || !m2.Closed {
		t.Errorf("KeyQuit should close without saving")
	}
}
