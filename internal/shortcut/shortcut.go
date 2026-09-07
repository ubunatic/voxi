// Package shortcut manages Voxi's opt-in desktop shortcut.
package shortcut

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"ubunatic.com/voxi/internal/deps"
)

const (
	mediaSchema  = "org.gnome.settings-daemon.plugins.media-keys"
	customSchema = "org.gnome.settings-daemon.plugins.media-keys.custom-keybinding"
	ownedPath    = "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/voxi/"
	ownedName    = "Voxi Toggle Dictation"
	accelerator  = "<Super>x"
)

var quotedValue = regexp.MustCompile(`^'(.*)'$`)

// Setup installs the standard GNOME shortcut without replacing other settings.
func Setup(ctx context.Context, d deps.Dependencies) error {
	if err := supported(d); err != nil {
		return err
	}
	voxi, err := d.LookPath("voxi")
	if err != nil {
		return fmt.Errorf("resolve installed voxi executable: %w (run make install first)", err)
	}
	voxi, err = filepath.Abs(voxi)
	if err != nil {
		return fmt.Errorf("resolve voxi executable path: %w", err)
	}
	paths, err := customPaths(ctx, d)
	if err != nil {
		return err
	}
	wantCommand := voxi + " record toggle"
	for _, path := range paths {
		entry, err := readEntry(ctx, d, path)
		if err != nil {
			return err
		}
		if path == ownedPath {
			if entry.Name != ownedName && !isVoxiCommand(entry.Command) {
				return fmt.Errorf("GNOME shortcut path %s is already used by %q; remove or rename it in Settings, then retry", path, entry.Name)
			}
			if entry.Binding == accelerator && entry.Command == wantCommand && entry.Name == ownedName {
				fmt.Fprintln(d.Stdout, "Voxi shortcut is already configured: Super+X")
				return nil
			}
			continue
		}
		if isVoxiCommand(entry.Command) || entry.Name == ownedName {
			return fmt.Errorf("an existing Voxi shortcut was found at %s; remove it in GNOME Settings before running setup to avoid duplicates", path)
		}
		if sameAccelerator(entry.Binding, accelerator) {
			return fmt.Errorf("Super+X is already assigned to %q (%s); change or remove that shortcut in GNOME Settings, then retry", entry.Name, entry.Command)
		}
	}
	if conflict, err := builtinConflict(ctx, d); err != nil {
		return err
	} else if conflict != "" {
		return fmt.Errorf("Super+X is already assigned by GNOME (%s); clear it in Settings > Keyboard > View and Customize Shortcuts, then retry", conflict)
	}

	child := customSchema + ":" + ownedPath
	for key, value := range map[string]string{"name": ownedName, "command": wantCommand, "binding": accelerator} {
		if err := set(ctx, d, child, key, value); err != nil {
			resetEntry(ctx, d)
			return err
		}
	}
	paths = append(paths, ownedPath)
	if err := setPaths(ctx, d, paths); err != nil {
		resetEntry(ctx, d)
		return err
	}
	fmt.Fprintf(d.Stdout, "Configured Super+X to run %s\n", wantCommand)
	return nil
}

// Remove removes only the fixed shortcut entry owned by Voxi.
func Remove(ctx context.Context, d deps.Dependencies) error {
	if err := supported(d); err != nil {
		return err
	}
	paths, err := customPaths(ctx, d)
	if err != nil {
		return err
	}
	found := false
	for _, path := range paths {
		if path == ownedPath {
			found = true
		}
	}
	if !found {
		fmt.Fprintln(d.Stdout, "No Voxi-owned shortcut is configured.")
		return nil
	}
	entry, err := readEntry(ctx, d, ownedPath)
	if err != nil {
		return err
	}
	if entry.Name != ownedName || !isVoxiCommand(entry.Command) || !sameAccelerator(entry.Binding, accelerator) {
		return fmt.Errorf("refusing to remove modified shortcut at %s; inspect it in GNOME Settings", ownedPath)
	}
	kept := paths[:0]
	for _, path := range paths {
		if path != ownedPath {
			kept = append(kept, path)
		}
	}
	if err := setPaths(ctx, d, kept); err != nil {
		return err
	}
	if err := resetEntry(ctx, d); err != nil {
		return err
	}
	fmt.Fprintln(d.Stdout, "Removed the Voxi Super+X shortcut.")
	return nil
}

type entry struct{ Name, Command, Binding string }

func supported(d deps.Dependencies) error {
	if !strings.Contains(strings.ToLower(d.Getenv("XDG_CURRENT_DESKTOP")), "gnome") {
		return fmt.Errorf("shortcut setup supports GNOME desktops only (XDG_CURRENT_DESKTOP=%q)", d.Getenv("XDG_CURRENT_DESKTOP"))
	}
	if _, err := d.LookPath("gsettings"); err != nil {
		return fmt.Errorf("GNOME shortcut setup requires gsettings: %w", err)
	}
	return nil
}

func customPaths(ctx context.Context, d deps.Dependencies) ([]string, error) {
	out, err := d.RunOutput(ctx, "gsettings", "get", mediaSchema, "custom-keybindings")
	if err != nil {
		return nil, fmt.Errorf("read GNOME custom shortcuts: %w", err)
	}
	trim := strings.TrimSpace(out)
	if trim == "@as []" || trim == "[]" {
		return nil, nil
	}
	var paths []string
	for _, match := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(trim, -1) {
		paths = append(paths, match[1])
	}
	return paths, nil
}

func readEntry(ctx context.Context, d deps.Dependencies, path string) (entry, error) {
	var e entry
	for key, target := range map[string]*string{"name": &e.Name, "command": &e.Command, "binding": &e.Binding} {
		out, err := d.RunOutput(ctx, "gsettings", "get", customSchema+":"+path, key)
		if err != nil {
			return e, fmt.Errorf("read GNOME shortcut %s %s: %w", path, key, err)
		}
		*target = unquote(strings.TrimSpace(out))
	}
	return e, nil
}

func builtinConflict(ctx context.Context, d deps.Dependencies) (string, error) {
	for _, schema := range []string{"org.gnome.desktop.wm.keybindings", mediaSchema} {
		out, err := d.RunOutput(ctx, "gsettings", "list-recursively", schema)
		if err != nil {
			return "", fmt.Errorf("inspect GNOME shortcuts in %s: %w", schema, err)
		}
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(normalizeAccel(line), normalizeAccel(accelerator)) {
				return line, nil
			}
		}
	}
	return "", nil
}

func set(ctx context.Context, d deps.Dependencies, schema, key, value string) error {
	if err := d.Run(ctx, "gsettings", "set", schema, key, value); err != nil {
		return fmt.Errorf("set GNOME shortcut %s: %w", key, err)
	}
	return nil
}
func setPaths(ctx context.Context, d deps.Dependencies, paths []string) error {
	sort.Strings(paths)
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = fmt.Sprintf("'%s'", p)
	}
	return set(ctx, d, mediaSchema, "custom-keybindings", "["+strings.Join(quoted, ", ")+"]")
}
func resetEntry(ctx context.Context, d deps.Dependencies) error {
	child := customSchema + ":" + ownedPath
	for _, key := range []string{"name", "command", "binding"} {
		if err := d.Run(ctx, "gsettings", "reset", child, key); err != nil {
			return fmt.Errorf("reset GNOME shortcut %s: %w", key, err)
		}
	}
	return nil
}
func unquote(s string) string {
	if m := quotedValue.FindStringSubmatch(s); m != nil {
		return strings.ReplaceAll(m[1], `\'`, `'`)
	}
	return s
}
func normalizeAccel(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "<primary>", "<control>"))
}
func sameAccelerator(a, b string) bool { return normalizeAccel(a) == normalizeAccel(b) }
func isVoxiCommand(command string) bool {
	f := strings.Fields(command)
	return len(f) == 3 && filepath.Base(f[0]) == "voxi" && f[1] == "record" && f[2] == "toggle"
}
