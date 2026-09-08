// Package audiolevel provides a live microphone input-level meter: holding
// a low-rate raw PCM capture open on the system's default audio source,
// computing a logarithmic dBFS amplitude from it, and applying VU-meter-
// style attack/decay ballistics for smooth on-screen display. It never
// writes captured audio anywhere.
//
// Ported from harnez's internal/usage/miclive.go (issues 244, 245, 257,
// 258, 260, 265, 270; see ../../harnez/docs/MicIndicators.md for the full
// research behind the constants and desktop-indicator behavior below) so
// both voxi and harnez can share one implementation instead of each
// maintaining their own copy of the same subprocess/RMS/ballistics
// pipeline.
package audiolevel

import (
	"context"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/audio"
)

// Metric selects which aggregate of the rolling sample window Meter.Update
// reports as the target level before ballistics are applied.
type Metric string

const (
	MetricMax  Metric = "max"
	MetricAvg  Metric = "avg"
	MetricMin  Metric = "min"
	MetricLive Metric = "live"
)

const (
	// DefaultSampleRate is deliberately low: a level meter only needs a
	// coarse amplitude reading, not audio fidelity, and a low rate keeps
	// the subprocess's CPU/bandwidth footprint negligible for a status
	// box that may sit open for a whole session.
	DefaultSampleRate = 8000
	// DefaultChunkBytes is ~50ms of mono s16le audio at DefaultSampleRate
	// (8000 samples/sec * 2 bytes/sample * 0.05s) — frequent enough (20 Hz)
	// for smooth, real-time VU meter response during speech.
	DefaultChunkBytes = 800
	// DefaultRetryInterval paces reconnect attempts after the capture
	// subprocess exits early (source unplugged, PipeWire restarted) or
	// never starts (permission denied).
	DefaultRetryInterval = 5 * time.Second
	// DefaultMinDBFS is the calibrated noise-floor cutoff for the live
	// meter. Digital full-scale is 0 dBFS; conversational speech recorded
	// with standard ADC gain typically sits at -46 to -35 dBFS. Mapping
	// [-60, 0] dBFS to [0, 100] places conversational voice in the 25%-45%
	// range on a visual bar while suppressing ambient room silence.
	DefaultMinDBFS = -60.0
	// DefaultDecayCutoff is the floor (0.5%) below which a decaying level
	// snaps cleanly to zero instead of trailing off asymptotically forever.
	DefaultDecayCutoff = 0.5
)

// Privacy-indicator exemption tags (harnez docs/MicIndicators.md §8
// "Strategy C"): GNOME Shell's volume.js skips any source-output whose
// application.id matches one of these from lighting the top-bar mic-in-use
// indicator (confirmed live on GNOME, issue 248); KDE Plasma's
// microphoneindicator.cpp instead skips any source-output tagged
// node.virtual=true (inferred from source, not yet independently
// canary-confirmed on a live KDE session). The two checks are additive and
// don't interfere, so ParecCommand sets both unconditionally.
const (
	SuppressApplicationID  = "org.gnome.VolumeControl"
	suppressPavucontrolID  = "org.PulseAudio.pavucontrol"
	suppressPeakDetectName = "Peak detect"
)

// ParecCommand builds a `parec` invocation that streams raw PCM16LE mono
// samples from the default source at sampleRate (DefaultSampleRate if <=0),
// tagged so GNOME's and KDE's mic-in-use indicators exempt it (see the
// Suppress* constants above). cmd.Cancel/WaitDelay are left for the caller
// to run via RunCapture, which sets them up for a clean SIGTERM teardown.
func ParecCommand(ctx context.Context, sampleRate int) *exec.Cmd {
	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}
	return exec.CommandContext(ctx, "parec",
		"--raw",
		"--format=s16le",
		"--rate="+strconv.Itoa(sampleRate),
		"--channels=1",
		"--latency-msec=20",
		"--process-time-msec=20",
		"--property=application.id="+SuppressApplicationID,
		"--property=node.virtual=true",
		"-d", "@DEFAULT_SOURCE@",
	)
}

// PwRecordCommand builds a `pw-record` invocation for PipeWire systems
// missing the `pulseaudio-utils` compat package (no `pactl`/`parec` on
// PATH). `--target` is left at its default ("auto"), which tracks the
// currently-configured default capture source the same way `parec -d
// @DEFAULT_SOURCE@` does. Unlike ParecCommand, no indicator-suppression
// tags are applied here: harnez's research did not confirm an equivalent
// property mechanism for pw-record's compact CLI.
func PwRecordCommand(ctx context.Context, sampleRate int) *exec.Cmd {
	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}
	return exec.CommandContext(ctx, "pw-record",
		"--raw",
		"--format=s16",
		"--rate="+strconv.Itoa(sampleRate),
		"--channels=1",
		"-",
	)
}

// IsGenuineRecording reports whether the long-form output of `pactl list
// source-outputs` contains at least one source-output that is NOT one of
// this package's own exempted meter streams (ParecCommand's tags) or
// GNOME's own volume-control/pavucontrol utilities — i.e. whether some
// other, real application is actually capturing audio right now. Callers
// must pass the long form (`pactl list source-outputs`), not `list short`:
// only the long form prints the application.id/media.name properties this
// needs to filter on.
func IsGenuineRecording(pactlListOutput string) bool {
	for _, block := range strings.Split(pactlListOutput, "Source Output #") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		if isExemptSourceOutputBlock(block) {
			continue
		}
		return true
	}
	return false
}

func isExemptSourceOutputBlock(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := cutPropertyValue(line, "application.id"); ok {
			if rest == SuppressApplicationID || rest == suppressPavucontrolID {
				return true
			}
		}
		if rest, ok := cutPropertyValue(line, "media.name"); ok {
			if rest == suppressPeakDetectName {
				return true
			}
		}
	}
	return false
}

// cutPropertyValue extracts the quoted value from a pactl property line
// shaped like `application.id = "org.gnome.VolumeControl"`.
func cutPropertyValue(line, key string) (string, bool) {
	rest, ok := strings.CutPrefix(line, key)
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	rest, ok = strings.CutPrefix(rest, "=")
	if !ok {
		return "", false
	}
	return strings.Trim(strings.TrimSpace(rest), `"`), true
}

// AmplitudeFromPCM16LE computes a 0-100 amplitude level from a buffer of
// signed 16-bit little-endian mono PCM samples, scaled logarithmically in
// dBFS across [minDBFS, 0] -> [0, 100] (DefaultMinDBFS if <=0 isn't a valid
// choice here — pass a real negative value, e.g. DefaultMinDBFS) so
// conversational speech registers visibly instead of being crushed into
// the bottom few percent by a linear scale. Reuses voxi's own
// audio.ComputeAudioRMS for the underlying RMS math rather than
// re-deriving it; an odd trailing byte (a chunk cut mid-sample) is dropped
// rather than causing a panic.
func AmplitudeFromPCM16LE(buf []byte, minDBFS float64) float64 {
	if len(buf) < 2 {
		return 0
	}
	if len(buf)%2 != 0 {
		buf = buf[:len(buf)-1]
	}
	rms := audio.ComputeAudioRMS(buf)
	if rms <= 0 {
		return 0
	}
	dBFS := 20 * math.Log10(float64(rms)/32768.0)
	level := (dBFS - minDBFS) / (0 - minDBFS) * 100.0
	if level > 100 {
		level = 100
	}
	if level < 0 || math.IsNaN(level) {
		level = 0
	}
	return level
}

// ApplyBallistics updates a displayed level with a new target: instant
// attack on rises, smooth exponential release on falls.
//
//	decayed = prev * exp(-dt / tau)
//
// Clamped to never drop below target, and snapped to 0.0 once decayed
// level falls below DefaultDecayCutoff. If decayDuration <= 0, ballistics
// are disabled and the target is returned immediately.
func ApplyBallistics(prev, target float64, dt, decayDuration time.Duration) float64 {
	if decayDuration <= 0 {
		return target
	}
	if target >= prev {
		return target
	}
	if dt <= 0 {
		return prev
	}
	tau := decayDuration.Seconds()
	decayed := prev * math.Exp(-dt.Seconds()/tau)
	if decayed < target {
		decayed = target
	}
	if decayed < DefaultDecayCutoff {
		return 0.0
	}
	return decayed
}

// Reading is one published sample from a Meter: Level is 0-100, and
// Available reports whether a real reading is currently flowing (false
// while (re)connecting, permanently false when capture can't work on this
// machine at all).
type Reading struct {
	Level     float64
	Available bool
}

type sample struct {
	ts    time.Time
	level float64
}

// Meter is the mutex-guarded handoff point between a background capture
// goroutine (the only writer, via Update) and any number of readers (via
// Snapshot) — typically a TUI redraw loop running on its own goroutine.
type Meter struct {
	mu             sync.Mutex
	reading        Reading
	samples        []sample
	lastUpdate     time.Time
	displayedLevel float64
}

// Update folds one new raw amplitude reading into the meter's rolling
// window, reduces the window to metric, applies ballistics against the
// previously displayed level, and returns (and stores) the resulting
// Reading. Passing available=false resets all rolling state, matching a
// capture subprocess restart.
func (m *Meter) Update(rawLevel float64, available bool, now time.Time, metric Metric, window, decay time.Duration) Reading {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !available {
		m.samples = nil
		m.lastUpdate = time.Time{}
		m.displayedLevel = 0
		m.reading = Reading{Available: false, Level: 0}
		return m.reading
	}

	m.samples = append(m.samples, sample{ts: now, level: rawLevel})

	cutoff := now.Add(-window)
	start := 0
	for start < len(m.samples) && m.samples[start].ts.Before(cutoff) {
		start++
	}
	if start > 0 {
		m.samples = m.samples[start:]
	}

	var targetLevel float64
	if len(m.samples) == 0 {
		targetLevel = rawLevel
	} else {
		switch metric {
		case MetricLive:
			targetLevel = rawLevel
		case MetricMin:
			targetLevel = m.samples[0].level
			for _, s := range m.samples[1:] {
				if s.level < targetLevel {
					targetLevel = s.level
				}
			}
		case MetricAvg:
			var sum float64
			for _, s := range m.samples {
				sum += s.level
			}
			targetLevel = sum / float64(len(m.samples))
		case MetricMax:
			fallthrough
		default:
			targetLevel = m.samples[0].level
			for _, s := range m.samples[1:] {
				if s.level > targetLevel {
					targetLevel = s.level
				}
			}
		}
	}

	var dt time.Duration
	if !m.lastUpdate.IsZero() {
		dt = now.Sub(m.lastUpdate)
	}
	m.displayedLevel = ApplyBallistics(m.displayedLevel, targetLevel, dt, decay)
	m.lastUpdate = now

	m.reading = Reading{Level: m.displayedLevel, Available: true}
	return m.reading
}

// Snapshot returns the most recently published Reading.
func (m *Meter) Snapshot() Reading {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reading
}

// Spec supplies the window-reduction metric and ballistics tuning that
// RunCapture/Manager should use for the *next* sample — a function rather
// than fixed values so a caller backed by live-reloadable configuration
// (e.g. harnez's YAML spec system) can change these between samples
// without restarting capture.
type Spec func() (metric Metric, window, decay time.Duration)

// RunCapture starts cmd (already configured to stream raw PCM16LE mono
// samples on stdout, e.g. via ParecCommand/PwRecordCommand) and drives the
// read/amplitude/ballistics pipeline into m until cmd exits or ctx is
// canceled. cmd.Cancel sends SIGTERM (not Go's default SIGKILL) so the
// capture subprocess gets a chance to release the audio device cleanly;
// WaitDelay bounds how long shutdown can take if it ignores that signal.
// If onSample is non-nil, it's invoked with every newly computed Reading.
func RunCapture(ctx context.Context, cmd *exec.Cmd, m *Meter, chunkBytes int, minDBFS float64, spec Spec, onSample func(Reading)) {
	if chunkBytes <= 0 {
		chunkBytes = DefaultChunkBytes
	}
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 2 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}

	buf := make([]byte, chunkBytes)
	for ctx.Err() == nil {
		n, err := io.ReadFull(stdout, buf)
		if n > 0 {
			level := AmplitudeFromPCM16LE(buf[:n], minDBFS)
			metric, window, decay := spec()
			reading := m.Update(level, true, time.Now(), metric, window, decay)
			if onSample != nil {
				onSample(reading)
			}
		}
		if err != nil {
			break
		}
	}
	_ = cmd.Wait()
}

// Manager owns the lifecycle of one background capture goroutine. Start it
// when a mic-level display becomes visible, Stop it when it's toggled off
// or the owning process exits — never leave one running unobserved.
type Manager struct {
	meter  Meter
	cancel context.CancelFunc
}

// StartManager begins capturing via buildCmd (e.g.
// func(ctx) *exec.Cmd { return ParecCommand(ctx, 0) }), deriving its
// lifetime from parent so canceling parent also stops capture. Returns a
// non-nil *Manager immediately even when buildCmd is nil (meaning live
// capture structurally can't work here — no supported backend found) so
// callers can treat Stop/Snapshot uniformly; Snapshot then always reports
// Reading{Available: false}.
func StartManager(parent context.Context, buildCmd func(context.Context) *exec.Cmd, chunkBytes int, minDBFS float64, retryInterval time.Duration, spec Spec, onSample func(Reading)) *Manager {
	ctx, cancel := context.WithCancel(parent)
	mgr := &Manager{cancel: cancel}
	if buildCmd == nil {
		return mgr
	}
	go runManager(ctx, &mgr.meter, buildCmd, chunkBytes, minDBFS, retryInterval, spec, onSample)
	return mgr
}

// runManager holds capture open for the life of ctx, restarting the
// subprocess with retryInterval backoff whenever it exits early — a source
// can come and go mid-session (USB mic unplugged, PipeWire restarted)
// without that being a reason to give up on the meter for the rest of the
// run.
func runManager(ctx context.Context, m *Meter, buildCmd func(context.Context) *exec.Cmd, chunkBytes int, minDBFS float64, retryInterval time.Duration, spec Spec, onSample func(Reading)) {
	if retryInterval <= 0 {
		retryInterval = DefaultRetryInterval
	}
	for ctx.Err() == nil {
		RunCapture(ctx, buildCmd(ctx), m, chunkBytes, minDBFS, spec, onSample)
		if ctx.Err() != nil {
			return
		}
		metric, window, decay := spec()
		m.Update(0, false, time.Now(), metric, window, decay)
		if onSample != nil {
			onSample(Reading{Available: false})
		}
		waitOrDone(ctx, retryInterval)
	}
}

func waitOrDone(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Stop tears down the capture subprocess (if any) and its reader
// goroutine. Safe to call on a nil *Manager and safe to call more than
// once.
func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.cancel()
}

// Snapshot returns the most recently published reading. Safe on nil.
func (m *Manager) Snapshot() Reading {
	if m == nil {
		return Reading{}
	}
	return m.meter.Snapshot()
}
