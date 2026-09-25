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
	mediaSchema   = "org.gnome.settings-daemon.plugins.media-keys"
	customSchema  = "org.gnome.settings-daemon.plugins.media-keys.custom-keybinding"
	ownedPath     = "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/voxi/"
	ownedName     = "Voxi Toggle Dictation"
	primaryPath   = "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/voxi-read-primary/"
	clipboardPath = "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/voxi-read-clipboard/"
)

var ownedEntries = []struct{ path, name, binding, command string }{
	{ownedPath, ownedName, "<Super>x", "record toggle"},
	{primaryPath, "Voxi Read Primary Selection", "<Super>y", "say --interrupt --from primary"},
	{clipboardPath, "Voxi Read Clipboard", "<Shift><Super>y", "say --interrupt --from clipboard"},
}

var quotedValue = regexp.MustCompile(`^'(.*)'$`)

// Setup installs the standard GNOME shortcut without replacing other settings.
// Force permits an accelerator conflict but never edits the conflicting setting.
func Setup(ctx context.Context, d deps.Dependencies, force bool) error {
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
	owned := make(map[string]struct{}, len(ownedEntries))
	for _, item := range ownedEntries {
		owned[item.path] = struct{}{}
	}
	for _, path := range paths {
		entry, err := readEntry(ctx, d, path)
		if err != nil {
			return err
		}
		if _, ok := owned[path]; ok {
			if !isOwnedEntry(entry, path, voxi) {
				return fmt.Errorf("GNOME shortcut path %s is already used by %q; remove or rename it in Settings, then retry", path, entry.Name)
			}
			continue
		}
		if isVoxiCommand(entry.Command) || entry.Name == ownedName || strings.HasPrefix(entry.Name, "Voxi Read ") {
			return fmt.Errorf("an existing Voxi shortcut was found at %s; remove it in GNOME Settings before running setup to avoid duplicates", path)
		}
		for _, item := range ownedEntries {
			if sameAccelerator(entry.Binding, item.binding) && !force {
				return fmt.Errorf("%s is already assigned to %q (%s); change or remove that shortcut in GNOME Settings, then retry", item.binding, entry.Name, entry.Command)
			}
		}
	}
	for _, item := range ownedEntries {
		if conflict, err := builtinConflict(ctx, d, item.binding); err != nil {
			return err
		} else if conflict != "" && !force {
			return fmt.Errorf("%s is already assigned by GNOME (%s); clear it in Settings > Keyboard > View and Customize Shortcuts, then retry", item.binding, conflict)
		}
	}
	allConfigured := true
	for _, item := range ownedEntries {
		present := false
		for _, path := range paths {
			if path == item.path {
				entry, err := readEntry(ctx, d, path)
				if err != nil {
					return err
				}
				present = isOwnedEntry(entry, path, voxi)
				break
			}
		}
		if !present {
			allConfigured = false
			break
		}
	}
	if allConfigured {
		fmt.Fprintln(d.Stdout, "Voxi shortcuts are already configured: Super+X, Super+Y, Super+Shift+Y")
		return nil
	}

	for _, item := range ownedEntries {
		child := customSchema + ":" + item.path
		for key, value := range map[string]string{"name": item.name, "command": voxi + " " + item.command, "binding": item.binding} {
			if err := set(ctx, d, child, key, value); err != nil {
				return err
			}
		}
		found := false
		for _, path := range paths {
			if path == item.path {
				found = true
				break
			}
		}
		if !found {
			paths = append(paths, item.path)
		}
	}
	if err := setPaths(ctx, d, paths); err != nil {
		return err
	}
	fmt.Fprintln(d.Stdout, "Configured Voxi shortcuts: Super+X, Super+Y, Super+Shift+Y")
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
	var found []string
	for _, path := range paths {
		for _, item := range ownedEntries {
			if path == item.path {
				found = append(found, path)
			}
		}
	}
	if len(found) == 0 {
		fmt.Fprintln(d.Stdout, "No Voxi-owned shortcut is configured.")
		return nil
	}
	for _, path := range found {
		entry, err := readEntry(ctx, d, path)
		if err != nil {
			return err
		}
		if !isOwnedEntry(entry, path, "") {
			return fmt.Errorf("refusing to remove modified shortcut at %s; inspect it in GNOME Settings", path)
		}
	}
	kept := paths[:0]
	for _, path := range paths {
		remove := false
		for _, ownedPath := range found {
			if path == ownedPath {
				remove = true
			}
		}
		if !remove {
			kept = append(kept, path)
		}
	}
	if err := setPaths(ctx, d, kept); err != nil {
		return err
	}
	for _, path := range found {
		if err := resetEntryAt(ctx, d, path); err != nil {
			return err
		}
	}
	fmt.Fprintln(d.Stdout, "Removed Voxi shortcuts.")
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

func builtinConflict(ctx context.Context, d deps.Dependencies, accelerator string) (string, error) {
	for _, schema := range []string{"org.gnome.desktop.wm.keybindings", mediaSchema} {
		out, err := d.RunOutput(ctx, "gsettings", "list-recursively", schema)
		if err != nil {
			return "", fmt.Errorf("inspect GNOME shortcuts in %s: %w", schema, err)
		}
		for _, line := range strings.Split(out, "\n") {
			for _, value := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(line, -1) {
				if sameAccelerator(value[1], accelerator) {
					return line, nil
				}
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
func resetEntryAt(ctx context.Context, d deps.Dependencies, path string) error {
	child := customSchema + ":" + path
	for _, key := range []string{"name", "command", "binding"} {
		if err := d.Run(ctx, "gsettings", "reset", child, key); err != nil {
			return fmt.Errorf("reset GNOME shortcut %s: %w", key, err)
		}
	}
	return nil
}

func isOwnedEntry(e entry, path, voxi string) bool {
	for _, item := range ownedEntries {
		if path != item.path || e.Name != item.name || !sameAccelerator(e.Binding, item.binding) {
			continue
		}
		fields := strings.Fields(e.Command)
		return len(fields) >= 3 && filepath.Base(fields[0]) == "voxi" && strings.Join(fields[1:], " ") == item.command &&
			(voxi == "" || fields[0] == voxi || filepath.Base(fields[0]) == "voxi")
	}
	return false
}
func unquote(s string) string {
	if m := quotedValue.FindStringSubmatch(s); m != nil {
		return strings.ReplaceAll(m[1], `\'`, `'`)
	}
	return s
}
func normalizeAccel(s string) string {
	s = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "<primary>", "<control>"))
	mods := regexp.MustCompile(`<[^>]+>`).FindAllString(s, -1)
	suffix := regexp.MustCompile(`(?:<[^>]+>)+`).ReplaceAllString(s, "")
	sort.Strings(mods)
	return strings.Join(mods, "") + suffix
}
func sameAccelerator(a, b string) bool { return normalizeAccel(a) == normalizeAccel(b) }
func isVoxiCommand(command string) bool {
	f := strings.Fields(command)
	return len(f) == 3 && filepath.Base(f[0]) == "voxi" && f[1] == "record" && f[2] == "toggle"
}
