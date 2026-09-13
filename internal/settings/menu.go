package settings

import (
	"fmt"
	"strconv"
	"strings"

	"ubunatic.com/voxi/internal/config"
)

// ItemKind classifies what kind of control a menu row represents.
type ItemKind int

const (
	ItemBool ItemKind = iota
	ItemChoice
	ItemAction
)

// ActionKind defines what an action row does.
type ActionKind int

const (
	ActionSave ActionKind = iota
	ActionCancel
)

// MenuItem represents one interactive setting row in the TUI menu.
type MenuItem struct {
	ID          string
	Title       string
	Description string
	Kind        ItemKind
	Action      ActionKind
	BoolValue   bool
	Choices     []string
	ChoiceIndex int
}

// MenuModel holds the active state of the interactive settings TUI menu.
type MenuModel struct {
	Items    []*MenuItem
	Cursor   int
	Modified bool
	Saved    bool
	Closed   bool
}

// DefaultCleanupModels lists common LLM cleanup model options.
var DefaultCleanupModels = []string{
	"qwen3-4b-instruct-2507-q4",
	"smollm3-3b-instruct-q4",
	"none",
}

// DefaultTypeDelays lists standard typing delay choices in milliseconds.
var DefaultTypeDelays = []int{0, 1, 5, 12}

// NewMenuModel initializes a new MenuModel populated from UserSettings.
func NewMenuModel(s *config.UserSettings, availableASRModels []string) *MenuModel {
	if s == nil {
		s = config.DefaultUserSettings()
	}

	// Prepare cleanup model choices
	cleanupChoices := append([]string{}, DefaultCleanupModels...)
	if s.CleanupModel != "" && !contains(cleanupChoices, s.CleanupModel) {
		cleanupChoices = append([]string{s.CleanupModel}, cleanupChoices...)
	}
	cleanupIdx := indexOf(cleanupChoices, s.CleanupModel)
	if cleanupIdx < 0 {
		cleanupIdx = 0
	}

	// Prepare ASR model choices
	asrChoices := []string{"cohere-transcribe-03-2026", "large-v3-turbo", "small.en", "base.en"}
	if len(availableASRModels) > 0 {
		asrChoices = append([]string{}, availableASRModels...)
	}
	if s.ASRModel != "" && !contains(asrChoices, s.ASRModel) {
		asrChoices = append([]string{s.ASRModel}, asrChoices...)
	}
	asrIdx := indexOf(asrChoices, s.ASRModel)
	if asrIdx < 0 {
		asrIdx = 0
	}

	// Prepare typing delay choices
	var delayChoices []string
	hasCurrentDelay := false
	for _, d := range DefaultTypeDelays {
		delayChoices = append(delayChoices, fmt.Sprintf("%dms", d))
		if d == s.TypeDelayMs {
			hasCurrentDelay = true
		}
	}
	if !hasCurrentDelay && s.TypeDelayMs >= 0 {
		delayChoices = append(delayChoices, fmt.Sprintf("%dms", s.TypeDelayMs))
	}
	delayIdx := indexOf(delayChoices, fmt.Sprintf("%dms", s.TypeDelayMs))
	if delayIdx < 0 {
		delayIdx = 0
	}

	items := []*MenuItem{
		{
			ID:          "llm_cleaner",
			Title:       "LLM Post-Processing Cleaner",
			Description: "Post-process raw transcripts with local LLM cleanup service",
			Kind:        ItemBool,
			BoolValue:   s.LLMCleaner,
		},
		{
			ID:          "cleanup_model",
			Title:       "Cleanup Model",
			Description: "Model identifier for local LLM cleanup service (e.g. lmcoder)",
			Kind:        ItemChoice,
			Choices:     cleanupChoices,
			ChoiceIndex: cleanupIdx,
		},
		{
			ID:          "asr_model",
			Title:       "ASR Model Engine",
			Description: "Speech recognition model (Cohere Transcribe default, Whisper via voxtype)",
			Kind:        ItemChoice,
			Choices:     asrChoices,
			ChoiceIndex: asrIdx,
		},
		{
			ID:          "type_delay_ms",
			Title:       "Keystroke Delay (type_delay_ms)",
			Description: "Inter-key delay for dotool synthetic typing injection",
			Kind:        ItemChoice,
			Choices:     delayChoices,
			ChoiceIndex: delayIdx,
		},
		{
			ID:          "dictation_history",
			Title:       "Dictation History",
			Description: "Persist transcribed utterances to local sensitive history store",
			Kind:        ItemBool,
			BoolValue:   s.DictationHistory,
		},
		{
			ID:          "modifier_gating",
			Title:       "Modifier Key Safety Gate",
			Description: "Pause typing injection when physical Ctrl/Alt/Super/Shift are held (voxi-modifierd)",
			Kind:        ItemBool,
			BoolValue:   s.ModifierGating,
		},
		{
			ID:          "save",
			Title:       "Save & Apply Changes",
			Description: "Write configuration to disk and apply settings",
			Kind:        ItemAction,
			Action:      ActionSave,
		},
		{
			ID:          "cancel",
			Title:       "Cancel / Exit",
			Description: "Exit settings without saving changes",
			Kind:        ItemAction,
			Action:      ActionCancel,
		},
	}

	return &MenuModel{
		Items:  items,
		Cursor: 0,
	}
}

// CurrentItem returns the currently selected MenuItem.
func (m *MenuModel) CurrentItem() *MenuItem {
	if len(m.Items) == 0 {
		return nil
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Items) {
		m.Cursor = len(m.Items) - 1
	}
	return m.Items[m.Cursor]
}

// MoveUp moves the cursor up by one row, wrapping around.
func (m *MenuModel) MoveUp() {
	if len(m.Items) == 0 {
		return
	}
	m.Cursor--
	if m.Cursor < 0 {
		m.Cursor = len(m.Items) - 1
	}
}

// MoveDown moves the cursor down by one row, wrapping around.
func (m *MenuModel) MoveDown() {
	if len(m.Items) == 0 {
		return
	}
	m.Cursor++
	if m.Cursor >= len(m.Items) {
		m.Cursor = 0
	}
}

// Next advances the current item's value forward or executes action.
func (m *MenuModel) Next() {
	item := m.CurrentItem()
	if item == nil {
		return
	}
	switch item.Kind {
	case ItemBool:
		item.BoolValue = !item.BoolValue
		m.Modified = true
	case ItemChoice:
		if len(item.Choices) > 0 {
			item.ChoiceIndex = (item.ChoiceIndex + 1) % len(item.Choices)
			m.Modified = true
		}
	case ItemAction:
		m.executeAction(item.Action)
	}
}

// Prev retreats the current item's value backward or executes action.
func (m *MenuModel) Prev() {
	item := m.CurrentItem()
	if item == nil {
		return
	}
	switch item.Kind {
	case ItemBool:
		item.BoolValue = !item.BoolValue
		m.Modified = true
	case ItemChoice:
		if len(item.Choices) > 0 {
			item.ChoiceIndex = (item.ChoiceIndex - 1 + len(item.Choices)) % len(item.Choices)
			m.Modified = true
		}
	case ItemAction:
		m.executeAction(item.Action)
	}
}

// ToggleOrNext toggles bool, advances choice, or activates action (Space or Enter).
func (m *MenuModel) ToggleOrNext() {
	m.Next()
}

// Save flags the model as saved and closed.
func (m *MenuModel) Save() {
	m.Saved = true
	m.Closed = true
}

// Cancel flags the model as closed without saving.
func (m *MenuModel) Cancel() {
	m.Closed = true
}

func (m *MenuModel) executeAction(action ActionKind) {
	switch action {
	case ActionSave:
		m.Save()
	case ActionCancel:
		m.Cancel()
	}
}

// HandleKey processes a single Key event against the menu model.
func (m *MenuModel) HandleKey(k Key) {
	switch k {
	case KeyUp:
		m.MoveUp()
	case KeyDown:
		m.MoveDown()
	case KeyLeft:
		m.Prev()
	case KeyRight:
		m.Next()
	case KeySpace, KeyEnter:
		m.ToggleOrNext()
	case KeySave:
		m.Save()
	case KeyQuit:
		m.Cancel()
	}
}

// ToUserSettings converts the current menu state into a populated UserSettings struct.
func (m *MenuModel) ToUserSettings(base *config.UserSettings) *config.UserSettings {
	res := config.DefaultUserSettings()
	if base != nil {
		*res = *base
	}

	for _, item := range m.Items {
		switch item.ID {
		case "llm_cleaner":
			res.LLMCleaner = item.BoolValue
		case "cleanup_model":
			if item.ChoiceIndex >= 0 && item.ChoiceIndex < len(item.Choices) {
				res.CleanupModel = item.Choices[item.ChoiceIndex]
			}
		case "asr_model":
			if item.ChoiceIndex >= 0 && item.ChoiceIndex < len(item.Choices) {
				res.ASRModel = item.Choices[item.ChoiceIndex]
			}
		case "type_delay_ms":
			if item.ChoiceIndex >= 0 && item.ChoiceIndex < len(item.Choices) {
				valStr := strings.TrimSuffix(item.Choices[item.ChoiceIndex], "ms")
				if ms, err := strconv.Atoi(valStr); err == nil {
					res.TypeDelayMs = ms
				}
			}
		case "dictation_history":
			res.DictationHistory = item.BoolValue
		case "modifier_gating":
			res.ModifierGating = item.BoolValue
		}
	}
	return res
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func indexOf(slice []string, val string) int {
	for i, item := range slice {
		if item == val {
			return i
		}
	}
	return -1
}
