package mic

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Kind names one way the mic setup can be broken.
type Kind string

const (
	// NoDefault: PipeWire has no default source, or it names an absent node.
	NoDefault Kind = "no_default"
	// StaleConfigured: the saved default names a device that is not connected.
	StaleConfigured Kind = "stale_configured"
	// BluetoothNoMic: the default is a plain Bluetooth input (not WirePlumber's
	// autoswitch loopback) whose card is in a playback-only profile (A2DP).
	BluetoothNoMic Kind = "bluetooth_no_mic"
	// DeadSignal: the running recording stalls or delivers digital silence.
	// Detected by the caller from the audio itself (see SignalMonitor).
	DeadSignal Kind = "dead_signal"
)

// Problem is one diagnosed fault.
type Problem struct {
	Kind   Kind
	Detail string
}

// Action is one repair step: a command plus the line the user is told.
type Action struct {
	Summary string
	Cmd     []string
}

// Diagnose finds faults visible in the PipeWire state alone.
func Diagnose(st State) []Problem {
	var out []Problem
	if st.ConfiguredSource != "" {
		if _, ok := st.Source(st.ConfiguredSource); !ok {
			out = append(out, Problem{StaleConfigured, "saved mic " + st.ConfiguredSource + " is not connected"})
		}
	}
	src, ok := st.Source(st.DefaultSource)
	if !ok {
		out = append(out, Problem{NoDefault, "no usable default mic"})
		return out
	}
	if src.Bluetooth && !src.Loopback {
		if dev, ok := st.Device(src.DeviceID); ok && dev.Active.Sources == 0 {
			out = append(out, Problem{BluetoothNoMic, dev.Description + " is in playback-only mode"})
		}
	}
	return out
}

// Plan turns problems into repair actions for the given state.
func Plan(st State, problems []Problem) []Action {
	var acts []Action
	needMic := false
	for _, p := range problems {
		switch p.Kind {
		case StaleConfigured:
			acts = append(acts, Action{
				Summary: "cleared saved mic " + st.ConfiguredSource,
				Cmd:     []string{"pw-metadata", "-d", "0", "default.configured.audio.source"},
			})
		case NoDefault, BluetoothNoMic:
			needMic = true
		case DeadSignal:
			// A silent Bluetooth mic means a broken link; a headset-mode switch
			// would not stick (WirePlumber restores A2DP once the stream closed).
			// A silent built-in mic is muted or broken hardware: nothing to switch.
			if src, ok := st.Source(st.DefaultSource); ok && src.Bluetooth {
				if a, ok := builtinDefault(st); ok {
					acts = append(acts, a)
				}
			}
		}
	}
	if needMic {
		acts = append(acts, headsetActions(st)...)
	}
	return acts
}

// Fallback makes a built-in mic the default when repairs left none usable.
func Fallback(st State) []Action {
	if a, ok := builtinDefault(st); ok {
		return []Action{a}
	}
	return nil
}

func headsetActions(st State) []Action {
	var acts []Action
	for _, dev := range st.Devices {
		if !dev.Bluetooth || !dev.IsHeadphone() || dev.Active.Sources > 0 {
			continue
		}
		p, ok := dev.HeadsetProfile()
		if !ok {
			continue
		}
		acts = append(acts, Action{
			Summary: dev.Description + " → headset mode",
			Cmd:     []string{"wpctl", "set-profile", strconv.Itoa(dev.ID), strconv.Itoa(p.Index)},
		})
	}
	return acts
}

// builtinDefault picks the non-Bluetooth source with an available input port
// and the highest port priority. WirePlumber ignores a saved default whose
// port is unavailable, so choosing such a source would change nothing.
func builtinDefault(st State) (Action, bool) {
	best, bestPrio, found := Source{}, 0, false
	for _, src := range st.Sources {
		if src.Bluetooth || src.Name == st.DefaultSource || !st.Usable(src) {
			continue
		}
		prio := 0
		if dev, ok := st.Device(src.DeviceID); ok {
			if r, ok := dev.inputRoute(src.ProfileDevice); ok {
				prio = r.Priority
			}
		}
		if !found || prio > bestPrio {
			best, bestPrio, found = src, prio, true
		}
	}
	if !found {
		return Action{}, false
	}
	return Action{
		Summary: "default mic → " + best.Description,
		Cmd:     []string{"wpctl", "set-default", strconv.Itoa(best.ID)},
	}, true
}

func urgent(problems []Problem) bool {
	for _, p := range problems {
		if p.Kind != StaleConfigured {
			return true
		}
	}
	return false
}

// Report says what Doctor found and did.
type Report struct {
	Problems  []Problem
	Applied   []Action
	Remaining []Problem
}

// Found reports whether any problem was seen.
func (r Report) Found() bool { return len(r.Problems) > 0 }

// Changed reports whether any setting was changed.
func (r Report) Changed() bool { return len(r.Applied) > 0 }

// Summary is the notification body.
func (r Report) Summary() string {
	var b strings.Builder
	if r.Changed() {
		b.WriteString("Mic repaired: ")
		for i, a := range r.Applied {
			if i > 0 {
				b.WriteString("; ")
			}
			b.WriteString(a.Summary)
		}
		b.WriteString(".")
	} else {
		b.WriteString("Mic problem: ")
		b.WriteString(r.Problems[0].Detail)
		b.WriteString(".")
	}
	if len(r.Remaining) > 0 && r.Changed() {
		b.WriteString(" Still: " + r.Remaining[0].Detail + ".")
	}
	b.WriteString(" Please record again.")
	return b.String()
}

// Doctor probes, diagnoses and repairs. Probe and Run are the host boundary.
type Doctor struct {
	Probe func(ctx context.Context) (State, error)
	Run   func(ctx context.Context, name string, args ...string) error
	Sleep func(time.Duration)
	// Settle bounds how long to wait for WirePlumber to apply a change.
	Settle time.Duration
}

// Find probes and reports the urgent problems, changing nothing.
func (doc Doctor) Find(ctx context.Context) (State, []Problem, error) {
	st, err := doc.Probe(ctx)
	if err != nil {
		return st, nil, err
	}
	probs := Diagnose(st)
	if !urgent(probs) {
		return st, nil, nil
	}
	return st, probs, nil
}

// Check diagnoses the current state plus any extra problems the caller saw
// (e.g. DeadSignal) and repairs what it can.
func (doc Doctor) Check(ctx context.Context, extra ...Problem) (Report, error) {
	st, err := doc.Probe(ctx)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Problems: append(Diagnose(st), extra...)}
	if !urgent(rep.Problems) {
		// A stale saved default alone is harmless while another mic works.
		return Report{}, nil
	}
	if err := doc.apply(ctx, &rep, Plan(st, rep.Problems)); err != nil {
		return rep, err
	}
	st, err = doc.settle(ctx, len(rep.Applied) > 0)
	if err != nil {
		return rep, err
	}
	rep.Remaining = Diagnose(st)
	if urgent(rep.Remaining) {
		if err := doc.apply(ctx, &rep, Fallback(st)); err != nil {
			return rep, err
		}
		if st, err = doc.settle(ctx, true); err != nil {
			return rep, err
		}
		rep.Remaining = Diagnose(st)
	}
	return rep, nil
}

func (doc Doctor) apply(ctx context.Context, rep *Report, acts []Action) error {
	for _, a := range acts {
		if err := doc.Run(ctx, a.Cmd[0], a.Cmd[1:]...); err != nil {
			return fmt.Errorf("mic: %s: %w", strings.Join(a.Cmd, " "), err)
		}
		rep.Applied = append(rep.Applied, a)
	}
	return nil
}

// settle re-probes until the state has no problems or Settle has passed.
func (doc Doctor) settle(ctx context.Context, changed bool) (State, error) {
	const step = 100 * time.Millisecond
	waited := time.Duration(0)
	for {
		st, err := doc.Probe(ctx)
		if err != nil || !changed || len(Diagnose(st)) == 0 || waited >= doc.Settle {
			return st, err
		}
		doc.Sleep(step)
		waited += step
	}
}
