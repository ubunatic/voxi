package mic

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// Headset is the opt-in Bluetooth headset setup behind `voxi mic headset`
// (issue 210): a WirePlumber drop-in that keeps the headset mic link awake, and
// the autoswitch setting that stops a headset from dropping to call quality.
// Both are changed only by Enable and undone only by Disable; doctor reads
// their status and never changes them.
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
	// DropinOwned: the file carries voxi's marker, so voxi may rewrite or remove it.
	DropinOwned
	// DropinForeign: a file exists without the marker; voxi never touches it.
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

// Render is the drop-in text for the spec values: the marker line, then the
// WirePlumber rule that sets the suspend timeout on each matched node.
func (h Headset) Render() string {
	var matches []string
	for _, m := range h.bm.NodeMatches {
		matches = append(matches, fmt.Sprintf(`      { node.name = %q }`, m))
	}
	return h.bm.DropinMarker + "\n" +
		"monitor.bluez.rules = [\n" +
		"  {\n" +
		"    matches = [\n" +
		strings.Join(matches, "\n") + "\n" +
		"    ]\n" +
		"    actions = {\n" +
		"      update-props = {\n" +
		"        session.suspend-timeout-seconds = " + strconv.Itoa(h.bm.SuspendTimeoutSeconds) + "\n" +
		"      }\n" +
		"    }\n" +
		"  }\n" +
		"]\n"
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
	if strings.Contains(string(data), h.bm.DropinMarker) {
		return DropinOwned, nil
	}
	return DropinForeign, nil
}

// EnableResult reports what Enable changed.
type EnableResult struct {
	// DropinChanged is true when the drop-in was written (new, or rewritten from an owned copy).
	DropinChanged bool
	Path          string
}

// Enable writes the keep-awake drop-in and turns autoswitch off. It refuses
// when WirePlumber is absent or stopped, and when the drop-in belongs to
// someone else. If the autoswitch write fails after the drop-in was written,
// the error says so, and the drop-in is kept.
func (h Headset) Enable(ctx context.Context) (EnableResult, error) {
	res := EnableResult{Path: h.DropinPath()}
	if _, err := h.d.LookPath("wpctl"); err != nil {
		return res, ErrNoWirePlumber
	}
	out, err := h.d.RunOutput(ctx, "systemctl", "--user", "is-active", "wireplumber.service")
	if err != nil || strings.TrimSpace(out) != "active" {
		return res, errors.New("WirePlumber is not running (systemctl --user is-active wireplumber.service)")
	}
	state, err := h.DropinStatus()
	if err != nil {
		return res, err
	}
	if state == DropinForeign {
		return res, fmt.Errorf("%s exists and is not voxi's (no %q line); left unchanged", res.Path, h.bm.DropinMarker)
	}
	want := h.Render()
	current, _ := h.d.ReadFile(res.Path)
	if state == DropinMissing || string(current) != want {
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
	return res, nil
}

// DisableResult reports what Disable changed.
type DisableResult struct {
	// Removed is true when voxi's drop-in was deleted.
	Removed bool
}

// Disable restores autoswitch (saved as true) and removes voxi's drop-in. It
// works while WirePlumber is stopped, but refuses a drop-in voxi does not own.
// Autoswitch is restored first, so a failure leaves the drop-in in place to retry.
func (h Headset) Disable(ctx context.Context) (DisableResult, error) {
	var res DisableResult
	if _, err := h.d.LookPath("wpctl"); err != nil {
		return res, ErrNoWirePlumber
	}
	state, err := h.DropinStatus()
	if err != nil {
		return res, err
	}
	if state == DropinForeign {
		return res, fmt.Errorf("%s is not voxi's (no %q line); left in place", h.DropinPath(), h.bm.DropinMarker)
	}
	if err := h.d.Run(ctx, "wpctl", "settings", "--save", h.bm.AutoswitchSetting, "true"); err != nil {
		return res, fmt.Errorf("autoswitch not restored, drop-in left in place: %w", err)
	}
	if state == DropinOwned {
		if err := h.d.Remove(h.DropinPath()); err != nil {
			return res, err
		}
		res.Removed = true
	}
	return res, nil
}

// StatusLine is the read-only summary doctor prints for the Bluetooth mic.
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
