package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"ubunatic.com/voxi/internal/config"
)

// ANSI terminal color and styling codes
const (
	ansiReset     = "\033[0m"
	ansiBold      = "\033[1m"
	ansiDim       = "\033[2m"
	ansiItalic    = "\033[3m"
	ansiGreen     = "\033[32m"
	ansiYellow    = "\033[33m"
	ansiCyan      = "\033[36m"
	ansiWhite     = "\033[37m"
	ansiGray      = "\033[90m"
	ansiHighlight = "\033[1;36m"
)

// RenderMenu renders the interactive TUI menu frame.
func RenderMenu(m *MenuModel, width int) string {
	if width <= 0 {
		width = 72
	}
	if width < 50 {
		width = 50
	}

	var buf bytes.Buffer

	// Title Box
	title := "VOXI SETTINGS"
	subtitle := "Interactive Feature Toggles & System Configuration"
	buf.WriteString("  " + ansiHighlight + "╭" + strings.Repeat("─", width-4) + "╮" + ansiReset + "\n")
	buf.WriteString("  " + ansiHighlight + "│" + ansiBold + ansiWhite + centerText(title, width-4) + ansiHighlight + "│" + ansiReset + "\n")
	buf.WriteString("  " + ansiHighlight + "│" + ansiDim + centerText(subtitle, width-4) + ansiHighlight + "│" + ansiReset + "\n")
	buf.WriteString("  " + ansiHighlight + "╰" + strings.Repeat("─", width-4) + "╯" + ansiReset + "\n\n")

	// Menu Rows
	for i, item := range m.Items {
		selected := i == m.Cursor

		cursorPrefix := "    "
		titleStyle := ""
		if selected {
			cursorPrefix = "  " + ansiHighlight + "❯ " + ansiReset
			titleStyle = ansiBold + ansiWhite
		}

		switch item.Kind {
		case ItemBool:
			valStr := ""
			if item.BoolValue {
				valStr = ansiGreen + "[✓ Enabled ]" + ansiReset
			} else {
				valStr = ansiGray + "[  Disabled]" + ansiReset
			}
			labelCol := padRight(item.Title, 36)
			buf.WriteString(fmt.Sprintf("%s%s%s%s    %s\n", cursorPrefix, titleStyle, labelCol, ansiReset, valStr))

		case ItemChoice:
			choiceVal := ""
			if item.ChoiceIndex >= 0 && item.ChoiceIndex < len(item.Choices) {
				choiceVal = item.Choices[item.ChoiceIndex]
			}
			choiceFormatted := ansiCyan + "‹ " + choiceVal + " ›" + ansiReset
			labelCol := padRight(item.Title, 36)
			buf.WriteString(fmt.Sprintf("%s%s%s%s    %s\n", cursorPrefix, titleStyle, labelCol, ansiReset, choiceFormatted))

		case ItemAction:
			actionStr := ""
			if item.Action == ActionSave {
				if selected {
					actionStr = ansiHighlight + ansiBold + "[ Save & Apply Changes ]" + ansiReset
				} else {
					actionStr = ansiGreen + "[ Save & Apply Changes ]" + ansiReset
				}
			} else {
				if selected {
					actionStr = ansiHighlight + ansiBold + "[ Cancel / Exit ]" + ansiReset
				} else {
					actionStr = ansiGray + "[ Cancel / Exit ]" + ansiReset
				}
			}
			// Extra newline before actions
			if i > 0 && m.Items[i-1].Kind != ItemAction {
				buf.WriteString("\n")
			}
			buf.WriteString(fmt.Sprintf("%s%s\n", cursorPrefix, actionStr))
		}
	}

	buf.WriteString("\n  " + ansiDim + strings.Repeat("─", width-4) + ansiReset + "\n")

	// Current Item Description
	curr := m.CurrentItem()
	desc := ""
	if curr != nil {
		desc = curr.Description
	}
	buf.WriteString("    " + ansiItalic + ansiGray + desc + ansiReset + "\n")
	buf.WriteString("  " + ansiDim + strings.Repeat("─", width-4) + ansiReset + "\n")

	// Controls Footer
	controls := "↑/↓/k/j: Navigate   Space/Enter: Toggle   ←/→: Cycle   s: Save   q: Quit"
	buf.WriteString("   " + ansiDim + controls + ansiReset + "\n")

	if m.Modified {
		buf.WriteString("   " + ansiYellow + "● Unsaved changes (press 's' to save and exit)" + ansiReset + "\n")
	} else {
		buf.WriteString("   " + ansiGray + "○ No pending changes" + ansiReset + "\n")
	}

	return buf.String()
}

// RenderSummary formats a clean plain-text summary suitable for non-TTY output.
func RenderSummary(s *config.UserSettings, home string) string {
	if s == nil {
		s = config.DefaultUserSettings()
	}

	var b bytes.Buffer
	b.WriteString("Voxi Configuration Summary:\n")

	cleanerStatus := "[ ] Disabled"
	if s.LLMCleaner {
		cleanerStatus = "[✓] Enabled"
	}
	b.WriteString(fmt.Sprintf("  %-22s %s\n", "LLM Cleaner:", cleanerStatus))
	b.WriteString(fmt.Sprintf("  %-22s %s\n", "Cleanup Model:", s.CleanupModel))
	b.WriteString(fmt.Sprintf("  %-22s %s\n", "ASR Model:", s.ASRModel))
	b.WriteString(fmt.Sprintf("  %-22s %dms\n", "Typing Delay:", s.TypeDelayMs))

	histStatus := "[ ] Disabled"
	if s.DictationHistory {
		histStatus = "[✓] Enabled"
	}
	b.WriteString(fmt.Sprintf("  %-22s %s\n", "Dictation History:", histStatus))

	modStatus := "[ ] Disabled"
	if s.ModifierGating {
		modStatus = "[✓] Enabled"
	}
	b.WriteString(fmt.Sprintf("  %-22s %s\n\n", "Modifier Gating:", modStatus))

	b.WriteString("Config Files:\n")
	b.WriteString(fmt.Sprintf("  YAML: %s\n", config.VoxiConfigYAMLPath(home)))
	b.WriteString(fmt.Sprintf("  Env:  %s\n", config.VoxiEnvPath(home)))
	b.WriteString(fmt.Sprintf("  TOML: %s\n", config.VoxtypeConfigPath(home)))

	return b.String()
}

// RenderJSON serializes UserSettings to indented JSON.
func RenderJSON(s *config.UserSettings) (string, error) {
	if s == nil {
		s = config.DefaultUserSettings()
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RenderSaveSuccess prints post-save instructions and paths.
func RenderSaveSuccess(home string) string {
	var b bytes.Buffer
	b.WriteString("✓ Configuration successfully saved:\n")
	b.WriteString(fmt.Sprintf("  • %s\n", config.VoxiConfigYAMLPath(home)))
	b.WriteString(fmt.Sprintf("  • %s\n", config.VoxiEnvPath(home)))
	b.WriteString(fmt.Sprintf("  • %s\n\n", config.VoxtypeConfigPath(home)))
	b.WriteString("Restart background services for changes to take effect:\n")
	b.WriteString("  systemctl --user restart voxi-agent.service\n")
	return b.String()
}

func padRight(str string, length int) string {
	if len(str) >= length {
		return str
	}
	return str + strings.Repeat(" ", length-len(str))
}

func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	left := (width - len(text)) / 2
	right := width - len(text) - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}
