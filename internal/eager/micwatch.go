package eager

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/mic"
	"ubunatic.com/voxi/spec"
)

// micWatch runs the issue 177 mic check alongside a recording: once from the
// PipeWire state at start, then on the audio itself for the whole recording
// (a Bluetooth link can fail mid-session). On a problem it calls abort (drop
// the session), waits until the recording stream is closed, then repairs and
// notifies. Repairing only after the stream closed matters: WirePlumber
// restores a Bluetooth card's previous profile when the last capture stream on
// it ends. The fast path never waits on it.
type micWatch struct {
	signal mic.SignalMonitor
	done   chan struct{}
	once   sync.Once

	// gain is the digital gain for captured frames, stored as float64 bits. It
	// starts at 1 and is set once by the async probe (issue 210 M4): the read
	// loop never waits on pw-dump, so frames read before resolved closes are
	// not boosted.
	gain        atomic.Uint64
	resolved    chan struct{}
	resolveOnce sync.Once
}

// Gain is the digital gain the read loop applies to each frame.
func (w *micWatch) Gain() float64 {
	return math.Float64frombits(w.gain.Load())
}

// resolve publishes the gain and marks the probe finished. It is safe to call
// more than once; only the first call sets the gain.
func (w *micWatch) resolve(g float64) {
	w.resolveOnce.Do(func() {
		w.gain.Store(math.Float64bits(g))
		close(w.resolved)
	})
}

// captureGainFor is the gain for a probed state: gain when the default source
// is a Bluetooth input matched by node_matches, else 1.
func captureGainFor(st mic.State, es *spec.EagerSpec, gain float64) float64 {
	if !st.DefaultIsBluetooth(es.BluetoothMic.NodeMatches) {
		return 1
	}
	return gain
}

// bluetoothGainFor is the Bluetooth default-source gain for one capture
// session (issue 210 M5): the bluetooth_mic_gain config override when it is in
// the spec's range, else the spec's gain_factor. It is read per session, so a
// change applies to the next session without a daemon restart. With no HOME it
// never reads config and uses the spec default.
func bluetoothGainFor(d deps.Dependencies, es *spec.EagerSpec) float64 {
	def := es.BluetoothMic.GainFactor
	home := d.Getenv("HOME")
	if home == "" {
		return def
	}
	s, err := config.LoadUserSettings(home)
	if err != nil {
		return def
	}
	g := s.EffectiveBluetoothMicGain(def)
	if config.ValidateBluetoothMicGain(g, es.BluetoothMic) != nil {
		return def
	}
	return g
}

// micWatchTick is how often the audio is judged; stalls deliver no frames, so
// judging cannot ride on Feed alone.
const micWatchTick = 250 * time.Millisecond

func startMicWatch(ctx context.Context, d deps.Dependencies, es *spec.EagerSpec, gain float64, bytesPerSec int, abort func()) *micWatch {
	w := &micWatch{
		signal:   mic.SignalMonitor{Window: es.MicDeadSignal(), BytesPerSec: bytesPerSec},
		done:     make(chan struct{}),
		resolved: make(chan struct{}),
	}
	w.gain.Store(math.Float64bits(1))
	if d.RunOutput == nil || d.Run == nil || d.LookPath == nil {
		w.resolve(1)
		return w
	}
	if _, err := d.LookPath("pw-dump"); err != nil {
		w.resolve(1) // not PipeWire (e.g. arecord on ALSA): nothing to repair
		return w
	}
	doc := mic.NewDoctor(d, es.MicSettle())
	logf := func(format string, args ...any) {
		if d.Stderr != nil {
			fmt.Fprintf(d.Stderr, "voxi: "+format+"\n", args...)
		}
	}
	repair := func(extra ...mic.Problem) {
		abort()
		<-w.done
		// The session context is canceled by now; the repair outlives it.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), es.MicCheckTimeout())
		defer cancel()
		rep, err := doc.Check(rctx, extra...)
		if err != nil {
			logf("mic repair: %v", err)
		}
		if !rep.Found() {
			rep.Problems = append(extra, mic.Problem{Kind: mic.NoDefault, Detail: "the mic was not usable"})
		}
		logf("%s", rep.Summary())
		mic.Notify(rctx, d, rep)
	}
	w.signal.Start(time.Now())
	go func() {
		cctx, cancel := context.WithTimeout(ctx, es.MicCheckTimeout())
		st, probs, err := doc.Find(cctx)
		cancel()
		if err != nil {
			w.resolve(1)
			logf("mic check: %v", err)
			return
		}
		w.resolve(captureGainFor(st, es, gain))
		if len(probs) > 0 {
			repair()
			return
		}
		if src, ok := st.Source(st.DefaultSource); ok && !src.Bluetooth {
			w.signal.SetZerosMeanDead(true)
		}
		tick := time.NewTicker(micWatchTick)
		defer tick.Stop()
		for {
			select {
			case now := <-tick.C:
				if w.signal.Check(now) {
					repair(mic.Problem{Kind: mic.DeadSignal, Detail: "the mic delivers no audio"})
					return
				}
			case <-w.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return w
}

// feed passes one captured frame to the signal monitor.
func (w *micWatch) feed(frame []byte) {
	w.signal.Feed(frame, time.Now())
}

// stop tells the watch that capture has ended and its stream is closed.
func (w *micWatch) stop() {
	w.once.Do(func() { close(w.done) })
}
