package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/history"
)

// CheckStatus represents the diagnostic outcome of a test probe.
type CheckStatus string

const (
	StatusPass CheckStatus = "PASS"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
	StatusSkip CheckStatus = "SKIP"
)

// DiagnosticItem holds results for a single configurable subsystem.
type DiagnosticItem struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Summary string      `json:"summary"`
	Detail  string      `json:"detail,omitempty"`
}

// DiagnosticReport aggregates the full diagnostic suite results.
type DiagnosticReport struct {
	Items    []DiagnosticItem `json:"items"`
	Passed   int              `json:"passed"`
	Warnings int              `json:"warnings"`
	Failed   int              `json:"failed"`
	Skipped  int              `json:"skipped"`
}

// RunDiagnostics executes all setting probes against the current host environment.
func RunDiagnostics(ctx context.Context, d deps.Dependencies, home string, s *config.UserSettings) *DiagnosticReport {
	if s == nil {
		s = config.DefaultUserSettings()
	}

	report := &DiagnosticReport{}

	// 1. ASR Transcription Engine
	asrItem := checkASREngine(d, s.ASRModel)
	report.addItem(asrItem)

	// 2. LLM Cleanup Service
	llmItem := checkLLMCleaner(ctx, s)
	report.addItem(llmItem)

	// 3. Typing & Keystroke Injection
	typingItem := checkTypingInjection(d, home, s.TypeDelayMs)
	report.addItem(typingItem)

	// 4. Modifier Key Gating Daemon
	modifierItem := checkModifierGating(ctx, d, s.ModifierGating)
	report.addItem(modifierItem)

	// 5. Dictation History Storage
	historyItem := checkDictationHistory(home, s.DictationHistory)
	report.addItem(historyItem)

	// 6. Systemd Background Daemon
	daemonItem := checkDaemonService(ctx, d)
	report.addItem(daemonItem)

	return report
}

func (r *DiagnosticReport) addItem(item DiagnosticItem) {
	r.Items = append(r.Items, item)
	switch item.Status {
	case StatusPass:
		r.Passed++
	case StatusWarn:
		r.Warnings++
	case StatusFail:
		r.Failed++
	case StatusSkip:
		r.Skipped++
	}
}

func checkASREngine(d deps.Dependencies, model string) DiagnosticItem {
	item := DiagnosticItem{Name: "ASR Engine"}
	if model == "" {
		model = "cohere-transcribe-03-2026"
	}

	if strings.HasPrefix(model, "cohere") {
		path, err := d.LookPath("crispasr")
		if err != nil {
			item.Status = StatusFail
			item.Summary = fmt.Sprintf("crispasr binary not found (required for %s)", model)
			item.Detail = "Install crispasr in ~/.local/bin or on $PATH"
			return item
		}
		item.Status = StatusPass
		item.Summary = fmt.Sprintf("%s (crispasr at %s)", model, path)
		return item
	}

	// Whisper models via voxtype
	path, err := d.LookPath("voxtype")
	if err != nil {
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("voxtype binary not found (required for %s)", model)
		item.Detail = "Install voxtype on $PATH"
		return item
	}

	if model == "large-v3-turbo" {
		// Check for GPU render node
		hasGPU := false
		if entries, err := os.ReadDir("/dev/dri"); err == nil {
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), "renderD") {
					hasGPU = true
					break
				}
			}
		}
		if !hasGPU {
			item.Status = StatusWarn
			item.Summary = fmt.Sprintf("%s requires GPU render node (/dev/dri/renderD*); cpu fallback will be used", model)
			return item
		}
		item.Status = StatusPass
		item.Summary = fmt.Sprintf("%s via voxtype (%s, GPU render node detected)", model, path)
		return item
	}

	item.Status = StatusPass
	item.Summary = fmt.Sprintf("%s via voxtype (%s)", model, path)
	return item
}

func checkLLMCleaner(ctx context.Context, s *config.UserSettings) DiagnosticItem {
	item := DiagnosticItem{Name: "LLM Cleaner"}
	baseURL := s.OpenAIBaseURL
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8734/v1"
	}
	model := s.CleanupModel
	if model == "" {
		model = "qwen3-4b-instruct-2507-q4"
	}

	probeURL := strings.TrimRight(baseURL, "/") + "/models"
	reqCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, probeURL, nil)
	isReachable := false
	if err == nil {
		client := &http.Client{Timeout: 1500 * time.Millisecond}
		resp, respErr := client.Do(req)
		if respErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				isReachable = true
			}
		}
	}

	if !s.LLMCleaner {
		if isReachable {
			item.Status = StatusSkip
			item.Summary = fmt.Sprintf("Disabled (service reachable at %s, model: %s)", baseURL, model)
		} else {
			item.Status = StatusSkip
			item.Summary = "Disabled"
		}
		return item
	}

	if isReachable {
		item.Status = StatusPass
		item.Summary = fmt.Sprintf("Enabled and connected to %s (model: %s)", baseURL, model)
		return item
	}

	item.Status = StatusWarn
	item.Summary = fmt.Sprintf("Enabled but endpoint unreachable at %s (fallback to raw ASR)", baseURL)
	item.Detail = "Start local lmcoder service or verify OPENAI_BASE_URL"
	return item
}

func checkTypingInjection(d deps.Dependencies, home string, typeDelayMs int) DiagnosticItem {
	item := DiagnosticItem{Name: "Typing Injection"}

	path, err := d.LookPath("dotool")
	if err != nil {
		item.Status = StatusFail
		item.Summary = "dotool binary not found on PATH (desktop typing injection disabled)"
		item.Detail = "Install dotool and ensure dotoold is running"
		return item
	}

	if typeDelayMs < 0 {
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("Invalid type_delay_ms (%d < 0)", typeDelayMs)
		return item
	}

	item.Status = StatusPass
	item.Summary = fmt.Sprintf("dotool at %s (type_delay_ms = %dms)", path, typeDelayMs)
	return item
}

func checkModifierGating(ctx context.Context, d deps.Dependencies, enabled bool) DiagnosticItem {
	item := DiagnosticItem{Name: "Modifier Gating"}
	if !enabled {
		item.Status = StatusSkip
		item.Summary = "Disabled"
		return item
	}

	// 1. Check if the world-readable modifier state file is present and readable
	const statePath = "/run/voxi/modifiers"
	if _, err := d.Stat(statePath); err == nil {
		if _, err := d.ReadFile(statePath); err == nil {
			item.Status = StatusPass
			item.Summary = "active (system daemon exporting to " + statePath + ")"
			return item
		}
	}

	// 2. Check system service status
	out, err := d.RunOutput(ctx, "systemctl", "is-active", "voxi-modifierd.service")
	trimmed := strings.TrimSpace(out)
	if err == nil && trimmed == "active" {
		item.Status = StatusPass
		item.Summary = "voxi-modifierd.service is active (system daemon)"
		return item
	}

	// 3. If service is not active, check binary and provide appropriate sudo instructions
	path, err := d.LookPath("voxi-modifierd")
	if err != nil {
		item.Status = StatusWarn
		item.Summary = "voxi-modifierd binary not found on PATH or ~/.local/bin"
		item.Detail = "Build and install with: make install && sudo voxi install"
		return item
	}

	item.Status = StatusWarn
	item.Summary = fmt.Sprintf("voxi-modifierd at %s, but service is not running", path)
	item.Detail = "Enable system daemon: sudo systemctl enable --now voxi-modifierd.service"
	return item
}

func checkDictationHistory(home string, enabled bool) DiagnosticItem {
	item := DiagnosticItem{Name: "Dictation History"}
	if !enabled {
		item.Status = StatusSkip
		item.Summary = "Disabled"
		return item
	}

	histPath := history.HistoryPath(home)
	dir := filepath.Dir(histPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("cannot create history directory %s: %v", dir, err)
		return item
	}

	testFile := filepath.Join(dir, ".voxi-write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("history directory %s not writable: %v", dir, err)
		return item
	}
	_ = os.Remove(testFile)

	item.Status = StatusPass
	item.Summary = fmt.Sprintf("storage writable at %s", histPath)
	return item
}

func checkDaemonService(ctx context.Context, d deps.Dependencies) DiagnosticItem {
	item := DiagnosticItem{Name: "Background Agent"}

	out, err := d.RunOutput(ctx, "systemctl", "--user", "is-active", "voxi-agent.service")
	trimmed := strings.TrimSpace(out)
	if err != nil && trimmed == "" {
		item.Status = StatusSkip
		item.Summary = "systemd user daemon status unavailable or not installed"
		return item
	}

	switch trimmed {
	case "active":
		item.Status = StatusPass
		item.Summary = "voxi-agent.service is active (running)"
	case "inactive":
		item.Status = StatusWarn
		item.Summary = "voxi-agent.service is inactive (stopped)"
		item.Detail = "Start service with: systemctl --user start voxi-agent.service"
	case "failed":
		item.Status = StatusFail
		item.Summary = "voxi-agent.service failed"
		item.Detail = "Check logs with: journalctl --user -u voxi-agent.service -e"
	default:
		item.Status = StatusSkip
		item.Summary = fmt.Sprintf("voxi-agent.service status: %s", trimmed)
	}
	return item
}

// RenderDiagnosticReport formats the diagnostic results as human-readable text.
func RenderDiagnosticReport(r *DiagnosticReport) string {
	if r == nil {
		return "No diagnostic results\n"
	}

	var b strings.Builder
	b.WriteString("\nVoxi Configuration & Component Diagnostics:\n\n")

	for _, item := range r.Items {
		var icon, statusStr string
		switch item.Status {
		case StatusPass:
			icon = ansiGreen + "[✓]" + ansiReset
			statusStr = ansiGreen + "PASS" + ansiReset
		case StatusWarn:
			icon = ansiYellow + "[!]" + ansiReset
			statusStr = ansiYellow + "WARN" + ansiReset
		case StatusFail:
			icon = "\033[31m[✗]" + ansiReset
			statusStr = "\033[31mFAIL" + ansiReset
		case StatusSkip:
			icon = ansiGray + "[•]" + ansiReset
			statusStr = ansiGray + "SKIP" + ansiReset
		}

		b.WriteString(fmt.Sprintf("  %s %-20s %-12s %s\n", icon, item.Name+":", statusStr, item.Summary))
		if item.Detail != "" && item.Status != StatusPass && item.Status != StatusSkip {
			b.WriteString(fmt.Sprintf("      %s%s%s\n", ansiDim, item.Detail, ansiReset))
		}
	}

	b.WriteString(fmt.Sprintf("\nSummary: %d passed, %d warning(s), %d failed, %d skipped\n\n",
		r.Passed, r.Warnings, r.Failed, r.Skipped))

	return b.String()
}

// RenderDiagnosticJSON formats the diagnostic results as indented JSON.
func RenderDiagnosticJSON(r *DiagnosticReport) (string, error) {
	if r == nil {
		r = &DiagnosticReport{}
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
