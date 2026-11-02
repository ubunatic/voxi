package mic

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// Headset is the opt-in Bluetooth headset setup behind `voxi mic headset`
// (issue 210): a WirePlumber drop-in that keeps the headset mic link awake, the
// autoswitch setting that stops a headset from dropping to call quality, and
// the card profile pinned to the headset (HFP) profile that makes the mic
// available while autoswitch is off. Enable changes all three; Disable undoes
// them. Doctor reads the drop-in status and never changes anything.
//
// Ownership: the drop-in belongs to voxi only when its first line is the exact
// spec marker. Anything else is someone else's and is left alone unless the
// user adopts it explicitly (Enable with adopt).
//
// Autoswitch undo: the drop-in records "restore autoswitch to true" only when
// voxi itself turned autoswitch off. A host that already had it off keeps it
// off on undo.
type Headset struct {
	d  deps.Dependencies
	bm spec.BluetoothMicSpec
}

// NewHeadset binds the spec's bluetooth_mic section to the host dependencies.
func NewHeadset(d deps.Dependencies, bm spec.BluetoothMicSpec) Headset {
	return Headset{d: d, bm: bm}
}

// DropinState says whether the drop-in file is absent, voxi's, or someone else's.
type DropinState int

const (
	// DropinMissing: no drop-in at the path.
	DropinMissing DropinState = iota
	// DropinOwned: the first line is voxi's marker, so voxi may rewrite or remove it.
	DropinOwned
	// DropinForeign: a file exists whose first line is not the marker; voxi never
	// touches it unless the user adopts it.
	DropinForeign
)

// ErrNoWirePlumber is returned when wpctl is not on PATH.
var ErrNoWirePlumber = errors.New("WirePlumber is not installed (wpctl not found)")

// DropinPath is the drop-in location: $XDG_CONFIG_HOME, else $HOME/.config.
func (h Headset) DropinPath() string {
	base := h.d.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(h.d.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "wireplumber", "wireplumber.conf.d", h.bm.DropinName)
}

// Render is the drop-in text: the marker line, the autoswitch restore record
// when restore is "true", then the WirePlumber rule that sets the suspend
// timeout on each matched node.
func (h Headset) Render(restore string) string {
	var matches []string
	for _, m := range h.bm.NodeMatches {
		matches = append(matches, fmt.Sprintf(`      { node.name = %q }`, m))
	}
	record := ""
	if restore != "" {
		record = h.bm.RestoreRecordPrefix + restore + "\n"
	}
	return h.bm.DropinMarker + "\n" + record +
		"monitor.bluez.rules = [\n" +
		"  {\n" +
		"    matches = [\n" +
		strings.Join(matches, "\n") + "\n" +
		"    ]\n" +
		"    actions = {\n" +
		"      update-props = {\n" +
		"        session.suspend-timeout-seconds = " + fmt.Sprint(h.bm.SuspendTimeoutSeconds) + "\n" +
		"      }\n" +
		"    }\n" +
		"  }\n" +
		"]\n"
}

// classify reports ownership from the drop-in's bytes.
func (h Headset) classify(data []byte) DropinState {
	first, _, _ := strings.Cut(string(data), "\n")
	if strings.TrimRight(first, "\r") == h.bm.DropinMarker {
		return DropinOwned
	}
	return DropinForeign
}

// restoreRecord returns the value of the autoswitch restore line, or "".
func (h Headset) restoreRecord(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, h.bm.RestoreRecordPrefix); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// DropinStatus reads the drop-in and classifies it.
func (h Headset) DropinStatus() (DropinState, error) {
	data, err := h.d.ReadFile(h.DropinPath())
	if errors.Is(err, fs.ErrNotExist) {
		return DropinMissing, nil
	}
	if err != nil {
		return DropinMissing, err
	}
	return h.classify(data), nil
}

// autoswitchValue reads the current autoswitch setting. `wpctl settings KEY`
// prints "Value: <v> (Saved: <v>)"; the value is the word after "Value:".
func (h Headset) autoswitchValue(ctx context.Context) (string, error) {
	out, err := h.d.RunOutput(ctx, "wpctl", "settings", h.bm.AutoswitchSetting)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", h.bm.AutoswitchSetting, err)
	}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Value:"); ok {
			if fields := strings.Fields(rest); len(fields) > 0 {
				return fields[0], nil
			}
		}
	}
	return "", fmt.Errorf("cannot read %s: unexpected output %q", h.bm.AutoswitchSetting, out)
}

// EnableResult reports what Enable changed.
type EnableResult struct {
	// DropinChanged is true when the drop-in was written (new, or rewritten).
	DropinChanged bool
	Path          string
	// AutoswitchWasOn is true when Enable turned autoswitch off, so Disable will restore it.
	AutoswitchWasOn bool
	// PinnedCards are the Bluetooth cards moved to the headset profile.
	PinnedCards []string
}

// Enable writes the keep-awake drop-in, turns autoswitch off, and pins any
// Bluetooth card on the A2DP profile to the headset profile. It refuses when
// WirePlumber is absent or stopped, when the drop-in is not voxi's (unless
// adopt is set), and when autoswitch cannot be read (nothing is changed then).
// A failure after the drop-in is written names the state left behind; the
// drop-in is kept so `--off` can undo it.
func (h Headset) Enable(ctx context.Context, adopt bool) (EnableResult, error) {
	res := EnableResult{Path: h.DropinPath()}
	if _, err := h.d.LookPath("wpctl"); err != nil {
		return res, ErrNoWirePlumber
	}
	out, err := h.d.RunOutput(ctx, "systemctl", "--user", "is-active", "wireplumber.service")
	if err != nil || strings.TrimSpace(out) != "active" {
		return res, errors.New("WirePlumber is not running (systemctl --user is-active wireplumber.service)")
	}
	current, readErr := h.d.ReadFile(res.Path)
	state := DropinMissing
	if readErr == nil {
		state = h.classify(current)
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return res, readErr
	}
	if state == DropinForeign && !adopt {
		return res, fmt.Errorf("%s exists and is not voxi's (first line is not %q); left unchanged. "+
			"To take it over, run: voxi mic headset --adopt", res.Path, h.bm.DropinMarker)
	}

	// Keep an existing restore record: a re-run must not record the value voxi
	// itself set. Otherwise read autoswitch now, before changing anything.
	restore := ""
	if state == DropinOwned {
		restore = h.restoreRecord(current)
	}
	if state != DropinOwned || restore == "" {
		val, err := h.autoswitchValue(ctx)
		if err != nil {
			return res, fmt.Errorf("%w; nothing changed", err)
		}
		restore = ""
		if val == "true" {
			restore = "true"
		}
	}
	res.AutoswitchWasOn = restore == "true"

	want := h.Render(restore)
	if state == DropinMissing || state == DropinForeign || string(current) != want {
		if err := h.d.MkdirAll(filepath.Dir(res.Path), 0o755); err != nil {
			return res, err
		}
		if err := h.d.WriteFile(res.Path, []byte(want), 0o644); err != nil {
			return res, err
		}
		res.DropinChanged = true
	}
	if err := h.d.Run(ctx, "wpctl", "settings", "--save", h.bm.AutoswitchSetting, "false"); err != nil {
		return res, fmt.Errorf("drop-in written, but autoswitch was not turned off: %w", err)
	}
	cards, err := h.pinHeadsetCards(ctx)
	res.PinnedCards = cards
	if err != nil {
		return res, fmt.Errorf("drop-in written and autoswitch off, but the card profile was not pinned: %w", err)
	}
	return res, nil
}

// pinHeadsetCards moves each Bluetooth card that is on the A2DP profile to the
// headset profile, which is the only profile that provides the mic while
// autoswitch is off. A card without the headset profile is left as it is.
func (h Headset) pinHeadsetCards(ctx context.Context) ([]string, error) {
	if _, err := h.d.LookPath("pw-dump"); err != nil {
		return nil, nil // not PipeWire: nothing to pin
	}
	st, err := NewDoctor(h.d, 0).Probe(ctx)
	if err != nil {
		return nil, err
	}
	var pinned []string
	for _, dev := range st.Devices {
		if !dev.Bluetooth || dev.Active.Name != h.bm.A2DPProfile {
			continue
		}
		if !hasProfile(dev, h.bm.HeadsetProfile) {
			continue
		}
		if err := h.d.Run(ctx, "pactl", "set-card-profile", dev.Name, h.bm.HeadsetProfile); err != nil {
			return pinned, fmt.Errorf("pactl set-card-profile %s: %w", dev.Name, err)
		}
		pinned = append(pinned, dev.Name)
	}
	return pinned, nil
}

func hasProfile(dev Device, name string) bool {
	for _, p := range dev.Profiles {
		if p.Name == name {
			return true
		}
	}
	return false
}

// DisableResult reports what Disable changed.
type DisableResult struct {
	// Removed is true when voxi's drop-in was deleted.
	Removed bool
	// AutoswitchRestored is true when autoswitch was set back to true.
	AutoswitchRestored bool
}

// Disable removes voxi's drop-in and, only when the drop-in records that voxi
// turned autoswitch off, sets it back to true. It refuses a drop-in voxi does
// not own. It works while WirePlumber is stopped unless autoswitch has to be
// restored. Autoswitch is restored first, so a failure leaves the drop-in in
// place to retry.
func (h Headset) Disable(ctx context.Context) (DisableResult, error) {
	var res DisableResult
	state, err := h.DropinStatus()
	if err != nil {
		return res, err
	}
	switch state {
	case DropinMissing:
		return res, nil
	case DropinForeign:
		return res, fmt.Errorf("%s is not voxi's (first line is not %q); left in place", h.DropinPath(), h.bm.DropinMarker)
	}
	data, err := h.d.ReadFile(h.DropinPath())
	if err != nil {
		return res, err
	}
	if h.restoreRecord(data) == "true" {
		if _, err := h.d.LookPath("wpctl"); err != nil {
			return res, ErrNoWirePlumber
		}
		if err := h.d.Run(ctx, "wpctl", "settings", "--save", h.bm.AutoswitchSetting, "true"); err != nil {
			return res, fmt.Errorf("autoswitch not restored, drop-in left in place: %w", err)
		}
		res.AutoswitchRestored = true
	}
	if err := h.d.Remove(h.DropinPath()); err != nil {
		return res, err
	}
	res.Removed = true
	return res, nil
}

// StatusLine is the read-only summary doctor prints for the Bluetooth mic.
// The wording is fixed; tests compare it exactly.
func (h Headset) StatusLine() string {
	state, err := h.DropinStatus()
	switch {
	case err != nil:
		return "Bluetooth keep-awake: unknown (" + err.Error() + ")"
	case state == DropinMissing:
		return "Bluetooth keep-awake: off (enable with voxi mic headset)"
	case state == DropinForeign:
		return "Bluetooth keep-awake: not voxi's drop-in (left alone)"
	default:
		return "Bluetooth keep-awake: on"
	}
}
