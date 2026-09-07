package shortcut

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

type fakeSettings struct {
	paths    []string
	entries  map[string]entry
	builtins string
	calls    []string
}

func (f *fakeSettings) deps(out *bytes.Buffer) deps.Dependencies {
	return deps.Dependencies{
		Getenv: func(key string) string {
			if key == "XDG_CURRENT_DESKTOP" {
				return "ubuntu:GNOME"
			}
			return ""
		},
		LookPath: func(name string) (string, error) {
			if name == "voxi" {
				return "/opt/voxi/bin/voxi", nil
			}
			if name == "gsettings" {
				return "/usr/bin/gsettings", nil
			}
			return "", errors.New("not found")
		},
		RunOutput: func(_ context.Context, _ string, args ...string) (string, error) {
			if args[0] == "list-recursively" {
				return f.builtins, nil
			}
			if args[1] == mediaSchema {
				var q []string
				for _, p := range f.paths {
					q = append(q, fmt.Sprintf("'%s'", p))
				}
				return "[" + strings.Join(q, ", ") + "]", nil
			}
			path := strings.TrimPrefix(args[1], customSchema+":")
			e := f.entries[path]
			switch args[2] {
			case "name":
				return fmt.Sprintf("'%s'", e.Name), nil
			case "command":
				return fmt.Sprintf("'%s'", e.Command), nil
			default:
				return fmt.Sprintf("'%s'", e.Binding), nil
			}
		},
		Run: func(_ context.Context, _ string, args ...string) error {
			f.calls = append(f.calls, strings.Join(args, " "))
			return nil
		},
		Stdout: out,
	}
}

func TestSetupConstructsOwnedShortcut(t *testing.T) {
	f := &fakeSettings{entries: map[string]entry{}}
	var out bytes.Buffer
	if err := Setup(context.Background(), f.deps(&out)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"set " + customSchema + ":" + ownedPath + " name " + ownedName,
		"set " + customSchema + ":" + ownedPath + " command /opt/voxi/bin/voxi record toggle",
		"set " + customSchema + ":" + ownedPath + " binding " + accelerator,
		"set " + mediaSchema + " custom-keybindings ['" + ownedPath + "']",
	}
	for _, call := range want {
		if !contains(f.calls, call) {
			t.Errorf("missing call %q in %#v", call, f.calls)
		}
	}
}

func TestSetupIdempotent(t *testing.T) {
	f := &fakeSettings{paths: []string{ownedPath}, entries: map[string]entry{ownedPath: {Name: ownedName, Command: "/opt/voxi/bin/voxi record toggle", Binding: accelerator}}}
	var out bytes.Buffer
	if err := Setup(context.Background(), f.deps(&out)); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("idempotent setup wrote settings: %#v", f.calls)
	}
}

func TestSetupDetectsConflictsWithoutWrites(t *testing.T) {
	tests := []struct {
		name string
		f    *fakeSettings
	}{
		{"custom accelerator", &fakeSettings{paths: []string{"/custom/one/"}, entries: map[string]entry{"/custom/one/": {Name: "Other", Command: "/bin/other", Binding: "<Super>x"}}}},
		{"existing voxi", &fakeSettings{paths: []string{"/custom/one/"}, entries: map[string]entry{"/custom/one/": {Name: "Old Voxi", Command: "/old/voxi record toggle", Binding: "<Alt>x"}}}},
		{"built in accelerator", &fakeSettings{entries: map[string]entry{}, builtins: "org.example key ['<Super>x']"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := Setup(context.Background(), tt.f.deps(&out)); err == nil {
				t.Fatal("expected conflict")
			}
			if len(tt.f.calls) != 0 {
				t.Fatalf("conflict mutated settings: %#v", tt.f.calls)
			}
		})
	}
}

func TestRemovePreservesUnrelatedShortcuts(t *testing.T) {
	other := "/custom/other/"
	f := &fakeSettings{paths: []string{other, ownedPath}, entries: map[string]entry{ownedPath: {Name: ownedName, Command: "/different/location/voxi record toggle", Binding: "<SUPER>x"}, other: {Name: "Other", Command: "/bin/other", Binding: "<Alt>q"}}}
	var out bytes.Buffer
	if err := Remove(context.Background(), f.deps(&out)); err != nil {
		t.Fatal(err)
	}
	want := "set " + mediaSchema + " custom-keybindings ['" + other + "']"
	if !contains(f.calls, want) {
		t.Fatalf("removal did not preserve unrelated shortcut: %#v", f.calls)
	}
}

func TestRemoveRefusesModifiedOwnedPath(t *testing.T) {
	f := &fakeSettings{paths: []string{ownedPath}, entries: map[string]entry{ownedPath: {Name: "Other", Command: "/bin/other", Binding: accelerator}}}
	var out bytes.Buffer
	if err := Remove(context.Background(), f.deps(&out)); err == nil {
		t.Fatal("expected refusal")
	}
	if len(f.calls) != 0 {
		t.Fatalf("unsafe removal wrote settings: %#v", f.calls)
	}
}

func TestUnsupportedDesktopDoesNotReadOrWrite(t *testing.T) {
	f := &fakeSettings{entries: map[string]entry{}}
	d := f.deps(&bytes.Buffer{})
	d.Getenv = func(string) string { return "sway" }
	if err := Setup(context.Background(), d); err == nil {
		t.Fatal("expected unsupported desktop error")
	}
	if len(f.calls) != 0 {
		t.Fatalf("unsupported setup wrote settings: %#v", f.calls)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
