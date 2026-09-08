package monitor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"ubunatic.com/voxi/audiolevel"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// loadedActions returns the parsed spec/actions.yaml, cached after the first call.
// Panics only if the embedded spec is malformed — covered by
// spec.TestLoadActions, so this cannot happen in a build that passed `make check`.
var loadedActions = sync.OnceValue(func() *spec.ActionSpec {
	s, err := spec.LoadActions()
	if err != nil {
		panic(err)
	}
	return s
})

// loadedMonitorSpec returns the parsed spec/monitor.yaml, cached after the first call.
// Panics only if the embedded spec is malformed — covered by
// spec.TestLoadMonitor, so this cannot happen in a build that passed `make check`.
var loadedMonitorSpec = sync.OnceValue(func() *spec.MonitorSpec {
	s, err := spec.LoadMonitor()
	if err != nil {
		panic(err)
	}
	return s
})

// micLevelSpec tunes the live mic-level meter for the monitor TUI per
// spec/monitor.yaml's mic_level settings: a short window smooths jitter between
// individual audio chunks without lagging noticeably behind speech onset, and
// attack/decay ease the displayed level toward each new target in both
// directions (see audiolevel.ApplyBallisticsEased) — it never jumps straight to
// a new value in a single frame.
func micLevelSpec() (metric audiolevel.Metric, window, attack, decay time.Duration) {
	m := loadedMonitorSpec()
	return audiolevel.MetricMax, m.Window(), m.Attack(), m.Decay()
}

// buildMicCaptureCmd picks parec (preferred: tags the stream so desktop mic-in-use
// indicators exempt it) or falls back to pw-record, matching the capture backends
// voxi's own recording paths already rely on, at spec/monitor.yaml's mic_level.
// sample_rate_hz. Returns nil if neither is on PATH, meaning live mic-level capture
// structurally can't work here.
func buildMicCaptureCmd(d deps.Dependencies) func(context.Context) *exec.Cmd {
	rate := loadedMonitorSpec().MicLevel.SampleRateHz
	if _, err := d.LookPath("parec"); err == nil {
		return func(ctx context.Context) *exec.Cmd { return audiolevel.ParecCommand(ctx, rate) }
	}
	if _, err := d.LookPath("pw-record"); err == nil {
		return func(ctx context.Context) *exec.Cmd { return audiolevel.PwRecordCommand(ctx, rate) }
	}
	return nil
}

// ResourceSections defines which btop-style monitoring boxes are currently displayed.
type ResourceSections struct {
	Speed      bool
	Hardware   bool
	Transcript bool
	Daemons    bool
}

// DefaultResourceSections returns all sections enabled.
func DefaultResourceSections() ResourceSections {
	return ResourceSections{
		Speed:      true,
		Hardware:   true,
		Transcript: true,
		Daemons:    true,
	}
}

// ParseSections parses a comma-separated list of section names or letters.
func ParseSections(s string) ResourceSections {
	trimmed := strings.TrimSpace(strings.ToLower(s))
	if trimmed == "" || trimmed == "all" {
		return DefaultResourceSections()
	}
	sec := ResourceSections{}
	for _, p := range strings.Split(trimmed, ",") {
		p = strings.TrimSpace(p)
		switch p {
		case "s", "speed", "status", "voice", "v", "1":
			sec.Speed = true
		case "h", "hardware", "cpu", "gpu", "hw", "c", "g", "2":
			sec.Hardware = true
		case "t", "transcript", "sentences", "feed", "3":
			sec.Transcript = true
		case "d", "daemons", "procs", "health", "p", "4":
			sec.Daemons = true
		}
	}
	if !sec.Speed && !sec.Hardware && !sec.Transcript && !sec.Daemons {
		return DefaultResourceSections()
	}
	return sec
}

// RunWatchResources refreshes resource monitor live in terminal.
func RunWatchResources(ctx context.Context, d deps.Dependencies, interval time.Duration, initialSec ResourceSections) error {
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	oldState, err := exec.Command("stty", "-F", "/dev/tty", "-g").Output()
	if err == nil {
		_ = exec.Command("stty", "-F", "/dev/tty", "cbreak", "-echo").Run()
		defer func() {
			_ = exec.Command("stty", "-F", "/dev/tty", string(bytes.TrimSpace(oldState))).Run()
		}()
	}

	var secLock sync.Mutex
	sec := initialSec

	// Cache the terminal width and only refresh it on an actual resize
	// (SIGWINCH) instead of PrintVoiceResourceReport re-querying it via a
	// `stty` subprocess on every paint frame -- at 30fps that subprocess
	// spawn was measured costing more CPU than the mic-level meter itself.
	cachedTerminalWidth.Store(int32(getTerminalWidth()))
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go func() {
		for {
			select {
			case <-sigCtx.Done():
				return
			case <-winch:
				cachedTerminalWidth.Store(int32(getTerminalWidth()))
			}
		}
	}()

	redrawChan := make(chan struct{}, 1)
	requestRedraw := func() {
		select {
		case redrawChan <- struct{}{}:
		default:
		}
	}

	// keyDispatch resolves raw tty bytes to action ids per spec/actions.yaml — the
	// single source of truth for the monitor's hotkeys (see docs/Spec.md); this must
	// not hardcode key literals that could drift out of sync with what that file
	// documents and displays.
	keyDispatch := loadedActions().KeyDispatch()

	handleKey := func(b byte) {
		id, ok := keyDispatch[b]
		if !ok {
			return
		}
		secLock.Lock()
		switch id {
		case "speed":
			sec.Speed = !sec.Speed
			requestRedraw()
		case "hardware":
			sec.Hardware = !sec.Hardware
			requestRedraw()
		case "transcript":
			sec.Transcript = !sec.Transcript
			requestRedraw()
		case "daemons":
			sec.Daemons = !sec.Daemons
			requestRedraw()
		case "all":
			sec = DefaultResourceSections()
			requestRedraw()
		case "quit":
			secLock.Unlock()
			stop()
			return
		}
		secLock.Unlock()
	}

	tty, ttyErr := os.Open("/dev/tty")
	if ttyErr == nil {
		defer tty.Close()

		// keyDebounce absorbs bursts of injected keystrokes (e.g. dotool typing a
		// dictated sentence into this terminal because it happens to hold desktop
		// focus while the user dictates): a genuine single keypress fires only
		// after keyDebounce of silence follows it; two or more bytes arriving
		// closer together than that are treated as burst noise and dropped
		// entirely, rather than toggling/quitting the TUI one letter at a time.
		const keyDebounce = 150 * time.Millisecond
		keyEvents := make(chan byte, 64)
		go func() {
			defer close(keyEvents)
			inputBuf := make([]byte, 1)
			for {
				n, err := tty.Read(inputBuf)
				if err != nil || n == 0 {
					return
				}
				keyEvents <- inputBuf[0]
			}
		}()

		go func() {
			var pending byte
			havePending := false
			burst := false
			timer := time.NewTimer(keyDebounce)
			if !timer.Stop() {
				<-timer.C
			}
			for {
				select {
				case b, ok := <-keyEvents:
					if !ok {
						return
					}
					switch {
					case havePending:
						burst = true
						havePending = false
					case !burst:
						pending = b
						havePending = true
					}
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(keyDebounce)
				case <-timer.C:
					if havePending {
						handleKey(pending)
					}
					havePending = false
					burst = false
				case <-sigCtx.Done():
					return
				}
			}
		}()
	}

	micMgr := audiolevel.StartManager(sigCtx, buildMicCaptureCmd(d), loadedMonitorSpec().ChunkBytes(), audiolevel.DefaultMinDBFS, 0, micLevelSpec, nil)
	defer micMgr.Stop()

	// Data collection (systemctl/proc/sysfs reads, agent RPC — all slow and variable-
	// latency) runs on its own independent ticker into a cached report, so the paint
	// path below never blocks on or is triggered by a data fetch: it only ever reads
	// the last-collected snapshot plus the mic meter's own always-fresh Snapshot().
	var cacheLock sync.Mutex
	cachedReport := CollectVoiceResources(sigCtx, d, audiolevel.Reading{})

	go func() {
		collectTicker := time.NewTicker(interval)
		defer collectTicker.Stop()
		for {
			select {
			case <-sigCtx.Done():
				return
			case <-collectTicker.C:
				report := CollectVoiceResources(sigCtx, d, audiolevel.Reading{})
				cacheLock.Lock()
				cachedReport = report
				cacheLock.Unlock()
			}
		}
	}()

	fmt.Print("\033[?25l\033[2J")
	defer fmt.Print("\033[?25h\n")

	renderFrame := func() {
		cacheLock.Lock()
		report := cachedReport
		cacheLock.Unlock()
		_, _, attack, decay := micLevelSpec()
		mic := micMgr.Tick(time.Now(), attack, decay)
		report.MicLevel = mic.Level
		report.MicAvailable = mic.Available

		secLock.Lock()
		activeSec := sec
		secLock.Unlock()

		var buf bytes.Buffer
		buf.WriteString("\033[H")
		PrintVoiceResourceReport(&buf, report, activeSec)
		buf.WriteString("\033[J")
		_, _ = d.Stdout.Write(buf.Bytes())
	}

	renderFrame()

	// The paint ticker is independent of the collect ticker above (which drives the
	// slow CPU/GPU/process sampling at the user-chosen --interval) and of the mic
	// capture's own internal sampling rate: it only ever reads already-cached state,
	// so it can redraw much faster than either without adding any extra data-source
	// load. paintRate is spec/monitor.yaml's paint.fps rather than derived from
	// --interval, so the mic gauge stays smooth even when --interval is set slow to
	// reduce CPU/GPU polling.
	paintRate := loadedMonitorSpec().PaintInterval()
	if interval < paintRate {
		paintRate = interval
	}
	paintTicker := time.NewTicker(paintRate)
	defer paintTicker.Stop()

	for {
		select {
		case <-sigCtx.Done():
			return nil
		case <-redrawChan:
			renderFrame()
		case <-paintTicker.C:
			renderFrame()
		}
	}
}
