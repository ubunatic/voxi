package monitor

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"ubunatic.com/voxi/internal/asr"
)

var sparkRunes = []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// RenderSparkline converts a float slice into a smoothed Unicode sparkline.
func RenderSparkline(values []float64, maxVal float64) string {
	if len(values) == 0 {
		return "            "
	}
	if maxVal <= 0 {
		maxVal = 100.0
	}

	var sb strings.Builder
	for _, v := range values {
		if v <= 0 {
			sb.WriteString(" ")
			continue
		}
		idx := int((v / maxVal) * float64(len(sparkRunes)-1))
		if idx < 1 {
			idx = 1
		}
		if idx >= len(sparkRunes) {
			idx = len(sparkRunes) - 1
		}
		sb.WriteRune(sparkRunes[idx])
	}
	return sb.String()
}

// RenderSpeedGauge renders an 8-segment visual gauge of processing speed.
func RenderSpeedGauge(rtf float64) string {
	if rtf <= 0 {
		return "\x1b[32m[████████]\x1b[0m"
	}
	bars := 8
	filled := int((1.0 - (rtf * 0.8)) * float64(bars))
	if filled < 1 {
		filled = 1
	}
	if filled > bars {
		filled = bars
	}

	var sb strings.Builder
	sb.WriteString("\x1b[32m[")
	for i := 0; i < bars; i++ {
		if i < filled {
			sb.WriteString("█")
		} else {
			sb.WriteString("░")
		}
	}
	sb.WriteString("]\x1b[0m")
	return sb.String()
}

// levelChars are the single-glyph loudness steps RenderLevelChar picks from, lowest to
// highest, matching the block-height styling of the sparkline runes used elsewhere.
var levelChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// RenderLevelChar renders live mic input level (0-100) as a single colored glyph whose
// height/color track loudness, e.g. for use as a live-updating status icon in place of a
// static ●.
func RenderLevelChar(level float64) string {
	idx := int(level / 100.0 * float64(len(levelChars)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(levelChars) {
		idx = len(levelChars) - 1
	}

	color := "\x1b[32m"
	if level >= 90 {
		color = "\x1b[31m"
	} else if level >= 65 {
		color = "\x1b[33m"
	}

	return color + string(levelChars[idx]) + "\x1b[0m"
}

// FormatBytes formats byte sizes into human-readable strings.
func FormatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// FormatDuration formats duration into compact human format.
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// RuneDisplayWidth returns visual column width of a single rune in terminal cells (0, 1, or 2).
func RuneDisplayWidth(r rune) int {
	if r < 32 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	if r >= 0x0300 && r <= 0x036F {
		return 0
	}
	if (r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x231A && r <= 0x231B) ||
		(r >= 0x23E9 && r <= 0x23EC) ||
		(r >= 0x23F0 && r <= 0x23F3) ||
		(r >= 0x25FD && r <= 0x25FE) ||
		(r >= 0x2614 && r <= 0x2615) ||
		(r >= 0x2648 && r <= 0x2653) ||
		(r >= 0x267F && r <= 0x267F) ||
		(r >= 0x2693 && r <= 0x2693) ||
		(r >= 0x26A0 && r <= 0x26A1) ||
		(r >= 0x26AA && r <= 0x26AB) ||
		(r >= 0x26BD && r <= 0x26BE) ||
		(r >= 0x26C4 && r <= 0x26C5) ||
		(r >= 0x26CE && r <= 0x26CF) ||
		(r >= 0x26D4 && r <= 0x26D4) ||
		(r >= 0x26EA && r <= 0x26EA) ||
		(r >= 0x26F2 && r <= 0x26F3) ||
		(r >= 0x26F5 && r <= 0x26F5) ||
		(r >= 0x26FA && r <= 0x26FA) ||
		(r >= 0x26FD && r <= 0x26FD) ||
		(r >= 0x2702 && r <= 0x2702) ||
		(r >= 0x2705 && r <= 0x2705) ||
		(r >= 0x2708 && r <= 0x270D) ||
		(r >= 0x270F && r <= 0x270F) ||
		(r >= 0x2712 && r <= 0x2712) ||
		(r >= 0x2714 && r <= 0x2714) ||
		(r >= 0x2716 && r <= 0x2716) ||
		(r >= 0x271D && r <= 0x271D) ||
		(r >= 0x2721 && r <= 0x2721) ||
		(r >= 0x2728 && r <= 0x2728) ||
		(r >= 0x2733 && r <= 0x2734) ||
		(r >= 0x2744 && r <= 0x2744) ||
		(r >= 0x2747 && r <= 0x2747) ||
		(r >= 0x274C && r <= 0x274C) ||
		(r >= 0x274E && r <= 0x274E) ||
		(r >= 0x2753 && r <= 0x2755) ||
		(r >= 0x2757 && r <= 0x2757) ||
		(r >= 0x2763 && r <= 0x2764) ||
		(r >= 0x2795 && r <= 0x2797) ||
		(r >= 0x27A1 && r <= 0x27A1) ||
		(r >= 0x27B0 && r <= 0x27B0) ||
		(r >= 0x27BF && r <= 0x27BF) ||
		(r >= 0x2934 && r <= 0x2935) ||
		(r >= 0x2B05 && r <= 0x2B07) ||
		(r >= 0x2B1B && r <= 0x2B1C) ||
		(r >= 0x2B50 && r <= 0x2B50) ||
		(r >= 0x2B55 && r <= 0x2B55) ||
		(r >= 0x2E80 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE10 && r <= 0xFE19) ||
		(r >= 0xFE30 && r <= 0xFE6F) ||
		(r >= 0xFF01 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F000 && r <= 0x1F9FF) ||
		(r >= 0x1FA00 && r <= 0x1FAFF) {
		return 2
	}
	return 1
}

// StringDisplayWidth returns visual column width of a string ignoring ANSI color codes.
func StringDisplayWidth(s string) int {
	clean := asr.StripANSI(s)
	width := 0
	for _, r := range clean {
		width += RuneDisplayWidth(r)
	}
	return width
}

// TruncateLineANSI truncates a line to maxVisWidth visible columns while preserving ANSI escape sequences.
func TruncateLineANSI(s string, maxVisWidth int) string {
	if maxVisWidth <= 3 {
		return "..."
	}
	if StringDisplayWidth(s) <= maxVisWidth {
		return s
	}

	var sb strings.Builder
	visCount := 0
	inEscape := false
	runes := []rune(s)
	targetVis := maxVisWidth - 3

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\x1b' {
			inEscape = true
			sb.WriteRune(r)
			continue
		}
		if inEscape {
			sb.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}

		w := RuneDisplayWidth(r)
		if visCount+w <= targetVis {
			sb.WriteRune(r)
			visCount += w
		} else {
			break
		}
	}
	sb.WriteString("...\x1b[0m")
	return sb.String()
}

func getTerminalWidth() int {
	cmd := exec.Command("stty", "-F", "/dev/tty", "size")
	if out, err := cmd.Output(); err == nil {
		parts := strings.Fields(string(out))
		if len(parts) >= 2 {
			if cols, err := strconv.Atoi(parts[1]); err == nil && cols >= 60 {
				return cols
			}
		}
	}
	return 90
}

// BoxSpec defines a rendered bordered panel.
type BoxSpec struct {
	Title string
	Lines []string
	Width int
}

// RenderBoxLines renders a bordered box with rounded corners.
func RenderBoxLines(b BoxSpec) []string {
	var res []string

	var top strings.Builder
	top.WriteString("╭─ ")
	top.WriteString(b.Title)
	top.WriteString(" ")
	visTitleLen := StringDisplayWidth(b.Title) + 4
	rem := b.Width - visTitleLen - 1
	if rem < 1 {
		rem = 1
	}
	top.WriteString(strings.Repeat("─", rem))
	top.WriteString("╮")
	res = append(res, top.String())

	maxContentWidth := b.Width - 4
	if maxContentWidth < 10 {
		maxContentWidth = 10
	}

	for _, l := range b.Lines {
		var body strings.Builder
		body.WriteString("│ ")
		visLen := StringDisplayWidth(l)
		if visLen > maxContentWidth {
			l = TruncateLineANSI(l, maxContentWidth)
			visLen = StringDisplayWidth(l)
		}
		body.WriteString(l)
		pad := maxContentWidth - visLen
		if pad < 0 {
			pad = 0
		}
		body.WriteString(strings.Repeat(" ", pad))
		body.WriteString(" │")
		res = append(res, body.String())
	}

	var bot strings.Builder
	bot.WriteString("╰")
	bot.WriteString(strings.Repeat("─", b.Width-2))
	bot.WriteString("╯")
	res = append(res, bot.String())

	return res
}

// CombineSideBySide merges two rendered boxes side by side.
func CombineSideBySide(leftLines, rightLines []string) []string {
	maxL := len(leftLines)
	if len(rightLines) > maxL {
		maxL = len(rightLines)
	}
	var res []string
	for i := 0; i < maxL; i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}
		res = append(res, l+" "+r)
	}
	return res
}

// PrintVoiceResourceReport writes a formatted terminal report of voice resources using true btop grid.
func PrintVoiceResourceReport(w io.Writer, r VoiceResourceReport, sec ResourceSections) {
	totalWidth := getTerminalWidth()
	if totalWidth > 110 {
		totalWidth = 110
	}
	if totalWidth < 76 {
		totalWidth = 76
	}

	colWidthLeft := (totalWidth - 1) / 2
	colWidthRight := totalWidth - 1 - colWidthLeft

	modStr := r.ModifierStatus
	if modStr == "" {
		modStr = "\x1b[90moff\x1b[0m"
	}
	speedLines := []string{
		fmt.Sprintf("status:  %s (%s / %s)", formatRecordState(r.RecordStatus, r.MicLevel, r.MicAvailable), r.Mode, r.ActiveModel),
		fmt.Sprintf("engine:  \x1b[32m%s\x1b[0m  ·  mods: %s", r.GPUAccel, modStr),
	}
	if r.EagerMetrics != nil && r.EagerMetrics.TotalChunks > 0 {
		m := r.EagerMetrics
		if m.LastUtterance != nil {
			speedMultiplier := 0.0
			if m.LastUtterance.RTF > 0 {
				speedMultiplier = 1.0 / m.LastUtterance.RTF
			}
			gauge := RenderSpeedGauge(m.LastUtterance.RTF)
			speedLines = append(speedLines,
				fmt.Sprintf("speed:   \x1b[32;1m%4.1fx realtime\x1b[0m %s", speedMultiplier, gauge),
				fmt.Sprintf("lag:     \x1b[1m%.2fs\x1b[0m · %s audio (%d utt)",
					m.LastUtterance.TranscribeSecs,
					FormatDuration(time.Duration(m.TotalAudioSecs*float64(time.Second))),
					m.TotalChunks),
			)
		} else {
			speedLines = append(speedLines,
				"speed:   \x1b[32m---\x1b[0m (waiting for speech)",
				"lag:     ---",
			)
		}
	} else {
		speedLines = append(speedLines,
			"speed:   \x1b[32m---\x1b[0m (waiting for speech)",
			"lag:     ---",
		)
	}

	sysLoad := r.AvgCPULoad / 6.0
	sparkline := r.CPUSparkline
	if sparkline == "" {
		sparkline = "            "
	}
	gpuSpark := r.GPUSparkline
	if gpuSpark == "" {
		gpuSpark = "            "
	}

	uptimeStr := "0s"
	if r.ServiceUptime > 0 {
		uptimeStr = FormatDuration(r.ServiceUptime)
	}

	memStr := FormatBytes(r.ServiceMemory)
	vramStr := "487 MB VRAM"
	if r.VRAMUsedBytes > 0 && r.VRAMTotalBytes > 0 {
		vramStr = fmt.Sprintf("%s / %s VRAM", FormatBytes(r.VRAMUsedBytes), FormatBytes(r.VRAMTotalBytes))
	}

	hwLines := []string{
		fmt.Sprintf("cpu:  \x1b[1m%4.1f%%\x1b[0m  \x1b[36m[%s]\x1b[0m  avg %3.1f%% (1c)", r.LiveCPULoad, sparkline, r.AvgCPULoad),
		fmt.Sprintf("gpu:  \x1b[1m%4.1f%%\x1b[0m  \x1b[35m[%s]\x1b[0m  sys %3.2f%%", r.LiveGPULoad, gpuSpark, sysLoad),
		fmt.Sprintf("mem:  %s daemon   %s", memStr, vramStr),
		fmt.Sprintf("up:   %s (PID %d)", uptimeStr, r.ServicePID),
	}

	if sec.Speed && sec.Hardware {
		boxSpeed := BoxSpec{Title: actionBoxTitle("speed"), Lines: speedLines, Width: colWidthLeft}
		boxHW := BoxSpec{Title: actionBoxTitle("hardware"), Lines: hwLines, Width: colWidthRight}
		rendered := CombineSideBySide(RenderBoxLines(boxSpeed), RenderBoxLines(boxHW))
		for _, line := range rendered {
			fmt.Fprintln(w, line)
		}
	} else if sec.Speed {
		boxSpeed := BoxSpec{Title: actionBoxTitle("speed"), Lines: speedLines, Width: totalWidth}
		for _, line := range RenderBoxLines(boxSpeed) {
			fmt.Fprintln(w, line)
		}
	} else if sec.Hardware {
		boxHW := BoxSpec{Title: actionBoxTitle("hardware"), Lines: hwLines, Width: totalWidth}
		for _, line := range RenderBoxLines(boxHW) {
			fmt.Fprintln(w, line)
		}
	}

	if sec.Transcript {
		var transLines []string
		if r.EagerMetrics != nil && len(r.EagerMetrics.Recent) > 0 {
			limit := 4
			if len(r.EagerMetrics.Recent) < limit {
				limit = len(r.EagerMetrics.Recent)
			}
			for i := 0; i < limit; i++ {
				u := r.EagerMetrics.Recent[i]
				timeStr := u.Timestamp.Format("15:04:05")
				shortText := u.Text
				if len(shortText) > 60 {
					shortText = shortText[:57] + "..."
				}
				transLines = append(transLines,
					fmt.Sprintf("\x1b[36m%s\x1b[0m  \x1b[32m[%4.2fs]\x1b[0m  %q", timeStr, u.TranscribeSecs, shortText))
			}
		} else {
			transLines = append(transLines, "\x1b[90m(no transcriptions yet - speak with Super+X to dictate)\x1b[0m")
		}
		boxTrans := BoxSpec{Title: actionBoxTitle("transcript"), Lines: transLines, Width: totalWidth}
		for _, line := range RenderBoxLines(boxTrans) {
			fmt.Fprintln(w, line)
		}
	}

	if sec.Daemons {
		var daemonLines []string
		if len(r.Processes) == 0 {
			daemonLines = append(daemonLines, "\x1b[90mNo active voice processes running.\x1b[0m")
		} else {
			var procSummaries []string
			for _, p := range r.Processes {
				procSummaries = append(procSummaries, fmt.Sprintf("\x1b[1m%s\x1b[0m (PID %d, %s)", p.Name, p.PID, FormatBytes(p.RSSBytes)))
			}
			healthBadge := "\x1b[32m✓ clean\x1b[0m"
			if len(r.ZombieWarnings) > 0 {
				healthBadge = fmt.Sprintf("\x1b[31;1m⚠️ %s\x1b[0m", r.ZombieWarnings[0])
			}
			daemonLines = append(daemonLines, fmt.Sprintf("%s  ·  %s", strings.Join(procSummaries, "  ·  "), healthBadge))
		}
		boxDaemons := BoxSpec{Title: actionBoxTitle("daemons"), Lines: daemonLines, Width: totalWidth}
		for _, line := range RenderBoxLines(boxDaemons) {
			fmt.Fprintln(w, line)
		}
	}

	fmt.Fprintf(w, " %s  %s  %s  %s  \x1b[90m│\x1b[0m  %s  %s\n",
		formatActionBadge("speed", sec.Speed),
		formatActionBadge("hardware", sec.Hardware),
		formatActionBadge("transcript", sec.Transcript),
		formatActionBadge("daemons", sec.Daemons),
		formatActionLabel("all"),
		formatActionLabel("quit"))
}

// actionKeyAndLabel resolves an action's canonical display key (its first configured
// key in spec/actions.yaml) and short label, so box titles and footer badges can never
// drift out of sync with the spec that defines those hotkeys.
func actionKeyAndLabel(id string) (key, short string) {
	a := loadedActions().Actions[id]
	key = "?"
	if len(a.Keys) > 0 {
		key = a.Keys[0]
	}
	return key, a.Short
}

func actionBoxTitle(id string) string {
	key, _ := actionKeyAndLabel(id)
	return fmt.Sprintf("\x1b[1m[%s] %s\x1b[0m", key, loadedActions().Actions[id].Title)
}

func formatActionBadge(id string, active bool) string {
	key, short := actionKeyAndLabel(id)
	if active {
		return fmt.Sprintf("\x1b[1m[%s]\x1b[0m%s \x1b[32;1m●\x1b[0m", key, short[1:])
	}
	return fmt.Sprintf("\x1b[90m[%s]%s ○\x1b[0m", key, short[1:])
}

func formatActionLabel(id string) string {
	key, short := actionKeyAndLabel(id)
	return fmt.Sprintf("\x1b[1m[%s]%s\x1b[0m", key, short[1:])
}

// formatRecordState renders the daemon's recording status alongside its icon. While
// recording, the icon is replaced by the live mic loudness glyph itself (in place of a
// static ●), so the status line doubles as the level meter instead of carrying a
// separate level: row. spec/monitor.yaml's status_icon.always_show_loudness (issue 086)
// extends that swap to the idle state too, so the loudness glyph is always visible.
func formatRecordState(status string, micLevel float64, micAvailable bool) string {
	alwaysLoudness := loadedMonitorSpec().StatusIcon.AlwaysShowLoudness
	switch strings.ToLower(status) {
	case "recording":
		icon := "\x1b[31;1m●\x1b[0m"
		if micAvailable {
			icon = RenderLevelChar(micLevel)
		}
		return icon + " \x1b[31;1mrec\x1b[0m"
	case "transcribing":
		return "\x1b[33;1m⏳ transcribing\x1b[0m"
	case "idle":
		if alwaysLoudness && micAvailable {
			return RenderLevelChar(micLevel) + " \x1b[32midle\x1b[0m"
		}
		return "\x1b[32m○ idle\x1b[0m"
	default:
		return status
	}
}
