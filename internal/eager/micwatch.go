package eager

import (
	"context"
	"fmt"
	"sync"
	"time"

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
}

// micWatchTick is how often the audio is judged; stalls deliver no frames, so
// judging cannot ride on Feed alone.
const micWatchTick = 250 * time.Millisecond

func startMicWatch(ctx context.Context, d deps.Dependencies, es *spec.EagerSpec, bytesPerSec int, abort func()) *micWatch {
	w := &micWatch{
		signal: mic.SignalMonitor{Window: es.MicDeadSignal(), BytesPerSec: bytesPerSec},
		done:   make(chan struct{}),
	}
	if d.RunOutput == nil || d.Run == nil || d.LookPath == nil {
		return w
	}
	if _, err := d.LookPath("pw-dump"); err != nil {
		return w // not PipeWire (e.g. arecord on ALSA): nothing to repair
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
			logf("mic check: %v", err)
			return
		}
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
