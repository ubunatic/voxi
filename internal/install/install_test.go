package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testEffects(t *testing.T) (*Effects, *[]string) {
	t.Helper()
	home := t.TempDir()
	source := filepath.Join(home, "source-voxi")
	if err := os.WriteFile(source, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	var commands []string
	moduleDir := filepath.Join(home, "dotool-module")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dotoold", "dotoolc"} {
		if err := os.WriteFile(filepath.Join(moduleDir, name), []byte(name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	e := DefaultEffects()
	e.GOOS = "linux"
	e.Home = home
	e.Executable = func() (string, error) { return source, nil }
	e.RunOutput = func(_ context.Context, name string, _ ...string) (string, error) {
		if name == "go" {
			return moduleDir, nil
		}
		return "", errors.New("unexpected output command")
	}
	e.BuildModifier = func(_ context.Context, dir string) (string, error) {
		path := filepath.Join(dir, "voxi-modifierd")
		if err := os.WriteFile(path, []byte("modifier"), 0755); err != nil {
			return "", err
		}
		return path, nil
	}
	e.Run = func(_ context.Context, name string, args ...string) error {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		return nil
	}
	e.RunStdin = func(_ context.Context, stdin, name string, args ...string) error {
		commands = append(commands, strings.Join(append([]string{name}, args...), " ")+" [stdin:"+string(stdin)+"]")
		return nil
	}
	e.DownloadHTTP = func(_ context.Context, _, _ string) error {
		return errors.New("mock download disabled")
	}
	return &e, &commands
}

func TestInstallDefaultIsUserScopedAndIdempotent(t *testing.T) {
	e, commands := testEffects(t)
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(*commands, "\n"), "sudo") {
		t.Fatalf("default install invoked sudo: %v", *commands)
	}
	if got := len(*commands); got != 14 {
		t.Fatalf("commands = %d, want complete dependency/service sequence twice: %v", got, *commands)
	}
	if !strings.Contains((*commands)[0], "curl -fL") || !strings.Contains((*commands)[3], "go install") || !strings.Contains((*commands)[4], "daemon-reload") {
		t.Fatalf("unexpected first install sequence: %v", (*commands)[:8])
	}
	service, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/voxi-agent.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(service), "ExecStart=%h/.local/bin/voxi agent --daemon") {
		t.Fatalf("service is not self-contained: %s", service)
	}
	target, err := os.Readlink(filepath.Join(e.Home, "go/bin/voxi"))
	if err != nil {
		t.Fatalf("go/bin/voxi is not a symlink: %v", err)
	}
	if want := filepath.Join(e.Home, ".local/bin/voxi"); target != want {
		t.Fatalf("go/bin/voxi symlink = %q, want %q", target, want)
	}
	if !strings.Contains(out.String(), "modifier daemon] skipped") {
		t.Fatalf("missing optional phase report: %s", out.String())
	}
}

func TestInstallModifierdIsExplicitAndUsesSudo(t *testing.T) {
	e, commands := testEffects(t)
	e.LookPath = func(string) (string, error) { return "/tmp/voxi-modifierd", nil }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, true); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	servicePath := filepath.Join(e.Home, ".cache/voxi/install/voxi-modifierd.service")
	service, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(joined, "sudo install -m 0755 "+filepath.Join(e.Home, ".cache/voxi/install/voxi-modifierd")+" /usr/local/bin/voxi-modifierd") ||
		!strings.Contains(joined, "sudo install -m 0644 "+servicePath+" /etc/systemd/system/voxi-modifierd.service") || !strings.Contains(string(service), "ProtectSystem=strict") {
		t.Fatalf("missing privileged setup: %s", joined)
	}
}

func TestInstallStopsBeforePrivilegedPhaseWhenUserPhaseFails(t *testing.T) {
	e, commands := testEffects(t)
	e.Run = func(_ context.Context, name string, args ...string) error {
		*commands = append(*commands, strings.Join(append([]string{name}, args...), " "))
		if name == "systemctl" && strings.Contains(strings.Join(args, " "), "dotoold.service") {
			return errors.New("user manager unavailable")
		}
		return nil
	}
	var out strings.Builder
	err := Install(context.Background(), &out, *e, true)
	if err == nil || !strings.Contains(err.Error(), "install phase user service activation") || strings.Contains(strings.Join(*commands, "\n"), "sudo") {
		t.Fatalf("error = %v, commands = %v, output = %q", err, *commands, out.String())
	}
}

func TestInstallReportsPrivilegedFailureAfterUserSuccess(t *testing.T) {
	e, commands := testEffects(t)
	e.Run = func(_ context.Context, name string, args ...string) error {
		*commands = append(*commands, strings.Join(append([]string{name}, args...), " "))
		if name == "sudo" {
			return errors.New("permission denied")
		}
		return nil
	}
	var out strings.Builder
	err := Install(context.Background(), &out, *e, true)
	if err == nil || !strings.Contains(err.Error(), "install phase modifier daemon (privileged)") || !strings.Contains(out.String(), "failed") {
		t.Fatalf("error = %v, output = %q", err, out.String())
	}
}

func TestInstallReportsPhaseError(t *testing.T) {
	e, _ := testEffects(t)
	e.MkdirAll = func(path string, _ os.FileMode) error {
		if strings.Contains(path, ".config/systemd") {
			return errors.New("read-only home")
		}
		return os.MkdirAll(path, 0755)
	}
	var out strings.Builder
	err := Install(context.Background(), &out, *e, false)
	if err == nil || !strings.Contains(err.Error(), "install phase user service units") || !strings.Contains(out.String(), "failed") {
		t.Fatalf("error = %v, output = %q", err, out.String())
	}
}

func TestInstallCommandHelpExplainsSafetyBoundary(t *testing.T) {
	e, _ := testEffects(t)
	cmd := NewCommand(*e, &strings.Builder{})
	help := cmd.Long
	if !strings.Contains(help, "--modifierd") || !strings.Contains(help, "never invokes sudo") {
		t.Fatalf("help = %s", help)
	}
}

func TestInstallCachedArchiveSkipsDownload(t *testing.T) {
	e, commands := testEffects(t)
	// Create cached archive on disk
	archivePath := filepath.Join(e.Home, ".cache/voxi/crispasr-linux-x86_64.tar.gz")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, []byte("fake tar content"), 0644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	if strings.Contains(joined, "curl -fL") {
		t.Fatalf("expected curl to be skipped when archive is cached, got commands: %s", joined)
	}
}

func TestInstallDownloadFallsBackToResilientHTTPOnCurlFailure(t *testing.T) {
	e, commands := testEffects(t)
	fallbackCalled := false
	e.Run = func(_ context.Context, name string, args ...string) error {
		*commands = append(*commands, strings.Join(append([]string{name}, args...), " "))
		if name == "curl" {
			return errors.New("exit status 56")
		}
		return nil
	}
	e.DownloadHTTP = func(_ context.Context, url, target string) error {
		fallbackCalled = true
		return nil
	}
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatalf("expected install to succeed via fallback, got error: %v", err)
	}
	if !fallbackCalled {
		t.Fatal("expected fallback DownloadHTTP to be invoked when curl failed")
	}
}

func TestInstallDownloadFailureProvidesActionableRemediation(t *testing.T) {
	e, _ := testEffects(t)
	e.Run = func(_ context.Context, name string, args ...string) error {
		if name == "curl" {
			return errors.New("exit status 56")
		}
		if name == "tar" && len(args) > 0 && args[0] == "-tzf" {
			return errors.New("invalid archive")
		}
		return nil
	}
	e.DownloadHTTP = func(_ context.Context, _, _ string) error {
		return errors.New("HTTP range connection reset")
	}
	var out strings.Builder
	err := Install(context.Background(), &out, *e, false)
	if err == nil {
		t.Fatal("expected install to fail when download fails")
	}
	msg := err.Error()
	if !strings.Contains(msg, "download CrispASR failed") || !strings.Contains(msg, "exit status 56") || !strings.Contains(msg, "To resolve manually:") || !strings.Contains(msg, "voxi install") {
		t.Fatalf("expected detailed actionable error message, got: %s", msg)
	}
}



