package install

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/spec"
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
	e.Confirm = func(string) (bool, error) { return false, nil }
	e.RunInteractive = func(_ context.Context, name string, args ...string) error {
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

func TestInstallTTSDependenciesPromptsBeforeDnf(t *testing.T) {
	available := map[string]bool{"dnf": true}
	e := DefaultEffects()
	e.LookPath = func(name string) (string, error) {
		if available[name] {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	prompted := ""
	command := ""
	e.Confirm = func(message string) (bool, error) {
		prompted = message
		return true, nil
	}
	e.RunInteractive = func(_ context.Context, name string, args ...string) error {
		command = strings.Join(append([]string{name}, args...), " ")
		available["text2wave"] = true
		available["espeak-ng"] = true
		available["pw-play"] = true
		return nil
	}
	var out strings.Builder
	if err := installTTSDependencies(context.Background(), &out, e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompted, "sudo dnf") || !strings.Contains(prompted, "festival") {
		t.Fatalf("confirmation prompt = %q", prompted)
	}
	if command != "sudo dnf install festival espeak-ng pipewire-utils" {
		t.Fatalf("interactive package command = %q", command)
	}
	if strings.Contains(command, "-y") {
		t.Fatalf("package manager was passed automatic confirmation: %q", command)
	}
}

func TestInstallTTSDependenciesDeclineDoesNotInvokeSystemPackageManager(t *testing.T) {
	e := DefaultEffects()
	e.LookPath = func(name string) (string, error) {
		if name == "dnf" {
			return "/usr/bin/dnf", nil
		}
		return "", os.ErrNotExist
	}
	e.Confirm = func(string) (bool, error) { return false, nil }
	var invoked bool
	e.RunInteractive = func(context.Context, string, ...string) error {
		invoked = true
		return nil
	}
	var out strings.Builder
	if err := installTTSDependencies(context.Background(), &out, e); err != nil {
		t.Fatal(err)
	}
	if invoked || !strings.Contains(out.String(), "skipped") {
		t.Fatalf("invoked=%v output=%q; expected prompted skip", invoked, out.String())
	}
}

func TestInstallTTSDependenciesUsesAptPipeWirePackage(t *testing.T) {
	available := map[string]bool{"apt-get": true}
	e := DefaultEffects()
	e.LookPath = func(name string) (string, error) {
		if available[name] {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	e.Confirm = func(string) (bool, error) { return true, nil }
	var command string
	e.RunInteractive = func(_ context.Context, name string, args ...string) error {
		command = strings.Join(append([]string{name}, args...), " ")
		available["text2wave"] = true
		available["espeak-ng"] = true
		available["pw-play"] = true
		return nil
	}
	if err := installTTSDependencies(context.Background(), io.Discard, e); err != nil {
		t.Fatal(err)
	}
	if command != "sudo apt-get install festival espeak-ng pipewire-bin" {
		t.Fatalf("interactive package command = %q", command)
	}
}

func TestInstallNoTTSFlagPersistsConfigOptOut(t *testing.T) {
	e, commands := testEffects(t)
	var out strings.Builder
	cmd := NewCommand(*e, &out)
	cmd.SetArgs([]string{"--no-tts"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	settings, err := config.LoadUserSettings(e.Home)
	if err != nil {
		t.Fatal(err)
	}
	if settings.TTSEnabled {
		t.Fatal("--no-tts did not persist tts_enabled: false")
	}
	if strings.Contains(strings.Join(*commands, "\n"), "sudo dnf") || strings.Contains(strings.Join(*commands, "\n"), "sudo apt-get") {
		t.Fatalf("--no-tts invoked system package manager: %v", *commands)
	}
	if !strings.Contains(out.String(), "disabled by config") {
		t.Fatalf("missing TTS opt-out result: %s", out.String())
	}
}

func TestInstallHonorsConfigTTSOptOut(t *testing.T) {
	e, _ := testEffects(t)
	if err := config.SetTTSEnabled(e.Home, false); err != nil {
		t.Fatal(err)
	}
	e.Confirm = func(string) (bool, error) {
		t.Fatal("package confirmation requested with TTS disabled")
		return false, nil
	}
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tts_enabled: false") {
		t.Fatalf("installer did not report saved opt-out: %s", out.String())
	}
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
	if got := len(*commands); got != 24 {
		t.Fatalf("commands = %d, want complete dependency/service sequence twice: %v", got, *commands)
	}
	if !strings.Contains((*commands)[0], "crispasr-linux-x86_64") || !strings.Contains((*commands)[7], "go install") || !strings.Contains((*commands)[8], "daemon-reload") {
		t.Fatalf("unexpected first install sequence: %v", (*commands)[:8])
	}
	service, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/voxi-agent.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(service), "ExecStart=%h/.local/bin/voxi agent --daemon") {
		t.Fatalf("service is not self-contained: %s", service)
	}
	dotoold, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/dotoold.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dotoold), "ExecStart=%h/.local/bin/dotoold") {
		t.Fatalf("dotoold.service ExecStart does not match where installUserDependencies writes the wrapper script: %s", dotoold)
	}
	if !strings.Contains(string(dotoold), "DOTOOL_XKB_LAYOUT=us") {
		t.Fatalf("dotoold.service layout placeholder not substituted (localectl unavailable should fall back to us): %s", dotoold)
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

func TestInstallR2T2EnablesUnitAndUsesResolvedBinary(t *testing.T) {
	e, commands := testEffects(t)
	writeR2T2Models(t, e.Home)
	configDir := filepath.Join(e.Home, ".config", "voxi")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("asr_model: r2t2-confucius4\nllama_server_path: /opt/llama/bin/llama-server\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.LookPath = func(name string) (string, error) {
		if name == "text2wave" || name == "espeak-ng" || name == "pw-play" {
			return "/usr/bin/" + name, nil
		}
		if name != "/opt/llama/bin/llama-server" {
			t.Fatalf("LookPath(%q), want configured path", name)
		}
		return name, nil
	}
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	unit, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/voxi-r2t2.service"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ExecStart=/opt/llama/bin/llama-server",
		"-c 4096",
		"--port 18131",
		"MemoryMax=",
		"Restart=on-failure",
	} {
		if !strings.Contains(string(unit), want) {
			t.Fatalf("R2T2 unit missing %q: %s", want, unit)
		}
	}
	if strings.Contains(string(unit), "@LLAMA_SERVER_PATH@") {
		t.Fatalf("R2T2 unit retains a binary placeholder: %s", unit)
	}
	if !strings.Contains(strings.Join(*commands, "\n"), "systemctl --user enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit not enabled for selected loopback backend: %v", *commands)
	}
}

func TestInstallDisablesR2T2UnitWhenBackendIsNotSelected(t *testing.T) {
	e, commands := testEffects(t)
	e.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	if !strings.Contains(joined, "systemctl --user disable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit not disabled when backend is not selected: %v", *commands)
	}
	if !strings.Contains(out.String(), "[R2T2 service] disabled") {
		t.Fatalf("install did not explain why R2T2 is inactive: %s", out.String())
	}
	if !strings.Contains(out.String(), "fallback /usr/bin/llama-server") {
		t.Fatalf("install did not report fallback ExecStart path: %s", out.String())
	}
	unit, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/voxi-r2t2.service"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(unit), "@LLAMA_SERVER_PATH@") {
		t.Fatalf("inactive unit retains a binary placeholder: %s", unit)
	}
}

func TestInstallFindsLlamaServerOnPathWhenSettingIsEmpty(t *testing.T) {
	e, commands := testEffects(t)
	writeR2T2Models(t, e.Home)
	configDir := filepath.Join(e.Home, ".config", "voxi")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("asr_model: r2t2-confucius4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.LookPath = func(name string) (string, error) {
		if name == "text2wave" || name == "espeak-ng" || name == "pw-play" {
			return "/usr/bin/" + name, nil
		}
		if name != "llama-server" {
			t.Fatalf("LookPath(%q), want PATH lookup", name)
		}
		return "/opt/llama/bin/llama-server", nil
	}
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	unit, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/voxi-r2t2.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), "ExecStart=/opt/llama/bin/llama-server") {
		t.Fatalf("PATH resolved binary missing from unit: %s", unit)
	}
	if !strings.Contains(strings.Join(*commands, "\n"), "systemctl --user enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit not enabled with PATH binary: %v", *commands)
	}
}

func TestInstallRequiresLlamaServerOnlyForActiveLoopbackBackend(t *testing.T) {
	e, commands := testEffects(t)
	writeR2T2Models(t, e.Home)
	configDir := filepath.Join(e.Home, ".config", "voxi")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("asr_model: r2t2-confucius4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	var out strings.Builder
	err := Install(context.Background(), &out, *e, false)
	if err == nil || !strings.Contains(err.Error(), "llama_server_path") {
		t.Fatalf("Install error = %v, want actionable llama_server_path error", err)
	}
	if strings.Contains(strings.Join(*commands, "\n"), "enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit activated without a resolved binary: %v", *commands)
	}
}

func TestInstallDoesNotStartR2T2ForDifferentLoopbackPort(t *testing.T) {
	e, commands := testEffects(t)
	configDir := filepath.Join(e.Home, ".config", "voxi")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("asr_model: openai-transcribe-gemini\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	var out strings.Builder
	if err := Install(context.Background(), &out, *e, false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	if strings.Contains(joined, "enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit started for another loopback endpoint: %v", *commands)
	}
	if !strings.Contains(joined, "disable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit was not disabled: %v", *commands)
	}
}

func TestSameLoopbackEndpointRejectsDifferentPort(t *testing.T) {
	if SameLoopbackEndpoint("http://127.0.0.1:8090/v1", "http://127.0.0.1:18131/v1") {
		t.Fatal("different loopback port matched the R2T2 endpoint")
	}
	if !SameLoopbackEndpoint("http://127.0.0.1:18131/v1", "http://127.0.0.1:18131/v1") {
		t.Fatal("identical R2T2 endpoint did not match")
	}
}

func TestInstallR2T2RequiresBothModelFiles(t *testing.T) {
	e, commands := testEffects(t)
	modelsDir := filepath.Join(e.Home, ".cache", "voxi", "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "Confucius4-R2T2-Q4_K_M.gguf"), []byte("test model"), 0644); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(e.Home, ".config", "voxi")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("asr_model: r2t2-confucius4\nllama_server_path: /opt/llama/bin/llama-server\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.LookPath = func(string) (string, error) { return "/opt/llama/bin/llama-server", nil }
	var out strings.Builder
	err := Install(context.Background(), &out, *e, false)
	if err == nil || !strings.Contains(err.Error(), "mmproj-Confucius4-R2T2-Q8_0.gguf") {
		t.Fatalf("Install error = %v, want actionable missing mmproj error", err)
	}
	if strings.Contains(strings.Join(*commands, "\n"), "enable --now voxi-r2t2.service") {
		t.Fatalf("R2T2 unit activated with missing model files: %v", *commands)
	}
}

func writeR2T2Models(t *testing.T, home string) {
	t.Helper()
	modelsDir := filepath.Join(home, ".cache", "voxi", "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Confucius4-R2T2-Q4_K_M.gguf", "mmproj-Confucius4-R2T2-Q8_0.gguf"} {
		if err := os.WriteFile(filepath.Join(modelsDir, name), []byte("test model"), 0644); err != nil {
			t.Fatal(err)
		}
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
	if !strings.Contains(help, "--modifierd") || !strings.Contains(help, "--no-tts") || !strings.Contains(help, "asks before using sudo") {
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
	if strings.Contains(joined, "curl -fL -o "+archivePath) {
		t.Fatalf("expected CrispASR curl to be skipped when archive is cached, got commands: %s", joined)
	}
}

func TestInstallPiperUsesVendoredBinaryAndDefaultVoice(t *testing.T) {
	e, commands := testEffects(t)
	var links [][2]string
	e.Symlink = func(oldname, newname string) error {
		links = append(links, [2]string{oldname, newname})
		return nil
	}
	if err := installPiper(context.Background(), *e, filepath.Join(e.Home, ".local", "bin")); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	for _, expected := range []string{
		"piper_linux_x86_64.tar.gz",
		"https://github.com/rhasspy/piper/releases/latest/download/piper_linux_x86_64.tar.gz",
		"en_US-lessac-medium.onnx",
		"en_US-lessac-medium.onnx.json",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("commands missing %q: %s", expected, joined)
		}
	}
	wantLink := filepath.Join(e.Home, ".local", "lib", "voxi", "piper", "piper")
	wantTarget := filepath.Join(e.Home, ".local", "bin", "piper")
	found := false
	for _, link := range links {
		if link == [2]string{wantLink, wantTarget} {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing Piper symlink %q -> %q; links = %v", wantLink, wantTarget, links)
	}
}

func TestInstallPiperArm64Asset(t *testing.T) {
	e, commands := testEffects(t)
	e.GOARCH = "arm64"
	e.Symlink = func(oldname, newname string) error {
		return nil
	}
	if err := installPiper(context.Background(), *e, filepath.Join(e.Home, ".local", "bin")); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	if !strings.Contains(joined, "piper_linux_aarch64.tar.gz") {
		t.Fatalf("expected arm64 Piper download to use piper_linux_aarch64.tar.gz, got: %s", joined)
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

func ttsServeTestEffects(t *testing.T) (*Effects, *[]string) {
	t.Helper()
	e, commands := testEffects(t)
	e.LookPath = func(name string) (string, error) { return "", errors.New("not found") } // skip R2T2/TTS package phases
	e.Confirm = func(string) (bool, error) { return true, nil }
	e.AvailableMemoryMB = func() (int, error) { return 16384, nil }
	// The command mock only records invocations; simulate the two commands
	// installTTSServe's idempotency checks (git .git dir, venv python binary)
	// depend on actually existing on disk, so re-runs can observe them.
	e.Run = func(ctx context.Context, name string, args ...string) error {
		*commands = append(*commands, strings.Join(append([]string{name}, args...), " "))
		switch {
		case name == "git" && len(args) > 1 && args[0] == "clone":
			return os.MkdirAll(filepath.Join(args[2], ".git"), 0755)
		case name == "python3" && len(args) == 3 && args[0] == "-m" && args[1] == "venv":
			if err := os.MkdirAll(filepath.Join(args[2], "bin"), 0755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(args[2], "bin", "python"), []byte("python"), 0755)
		}
		return nil
	}
	return e, commands
}

func TestInstallTTSServeSkippedWithoutFlag(t *testing.T) {
	e, commands := ttsServeTestEffects(t)
	var out strings.Builder
	if err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[tts-serve] skipped") {
		t.Fatalf("missing skip report: %s", out.String())
	}
	if strings.Contains(strings.Join(*commands, "\n"), "tts-serve") {
		t.Fatalf("tts-serve commands ran without --tts-serve: %v", *commands)
	}
	if _, err := os.Stat(filepath.Join(e.Home, ".config/systemd/user/voxi-tts-serve.service")); !os.IsNotExist(err) {
		t.Fatalf("voxi-tts-serve.service should not be written without --tts-serve, stat err = %v", err)
	}
}

func TestInstallTTSServeDeclinedDoesNothing(t *testing.T) {
	e, commands := ttsServeTestEffects(t)
	e.Confirm = func(string) (bool, error) { return false, nil }
	var out strings.Builder
	if err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{TTSServe: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tts-serve setup declined; nothing installed") {
		t.Fatalf("missing decline report: %s", out.String())
	}
	if strings.Contains(strings.Join(*commands, "\n"), "git clone") {
		t.Fatalf("declined tts-serve setup ran git clone: %v", *commands)
	}
	if _, err := os.Stat(filepath.Join(e.Home, ".cache/voxi/tts-serve")); !os.IsNotExist(err) {
		t.Fatalf("declined tts-serve setup should not create a checkout, stat err = %v", err)
	}
}

func TestInstallTTSServeRunsPinnedSequenceAndWritesUnit(t *testing.T) {
	e, commands := ttsServeTestEffects(t)
	var out strings.Builder
	if err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{TTSServe: true}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	ttsSpec, err := spec.LoadTTS()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(e.Home, ".cache/voxi/tts-serve")
	for _, want := range []string{
		"git clone " + ttsSpec.TTSServeInstall.TTSServeRepo + " " + dir,
		"git -C " + dir + " fetch origin " + ttsSpec.TTSServeInstall.TTSServeCommit,
		"git -C " + dir + " checkout " + ttsSpec.TTSServeInstall.TTSServeCommit,
		"python3 -m venv " + filepath.Join(dir, ".venv"),
		filepath.Join(dir, ".venv/bin/pip") + " install git+" + ttsSpec.TTSServeInstall.ChatterboxRepo + "@" + ttsSpec.TTSServeInstall.ChatterboxCommit,
		filepath.Join(dir, ".venv/bin/pip") + " install " + filepath.Join(dir, "tts-engine-common") + " fastapi uvicorn loguru soundfile",
		"systemctl --user daemon-reload",
		"systemctl --user enable --now voxi-tts-serve.service",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing command %q in sequence: %v", want, *commands)
		}
	}
	unit, err := os.ReadFile(filepath.Join(e.Home, ".config/systemd/user/voxi-tts-serve.service"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"CHATTERBOX_HOST=127.0.0.1",
		"CHATTERBOX_PORT=8000",
		"CHATTERBOX_DEVICE=cpu",
		"ExecStart=%h/.cache/voxi/tts-serve/.venv/bin/python impl/server_chatterbox.py",
		"WorkingDirectory=%h/.cache/voxi/tts-serve",
	} {
		if !strings.Contains(string(unit), want) {
			t.Fatalf("tts-serve unit missing %q: %s", want, unit)
		}
	}
	if strings.Contains(string(unit), "@TTS_SERVE_PORT@") {
		t.Fatalf("tts-serve unit retains an unsubstituted placeholder: %s", unit)
	}
}

func TestInstallTTSServeIsIdempotentOnReruns(t *testing.T) {
	e, commands := ttsServeTestEffects(t)
	var out strings.Builder
	if err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{TTSServe: true}); err != nil {
		t.Fatal(err)
	}
	*commands = nil
	if err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{TTSServe: true}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*commands, "\n")
	if strings.Contains(joined, "git clone") {
		t.Fatalf("re-run should reuse the existing checkout instead of cloning again: %v", *commands)
	}
	if strings.Contains(joined, "python3 -m venv") {
		t.Fatalf("re-run should reuse the existing venv instead of recreating it: %v", *commands)
	}
	dir := filepath.Join(e.Home, ".cache/voxi/tts-serve")
	for _, want := range []string{
		"git -C " + dir + " fetch origin",
		"git -C " + dir + " checkout",
		filepath.Join(dir, ".venv/bin/pip") + " install git+",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("re-run should still fetch/checkout/install, missing %q: %v", want, *commands)
		}
	}
}

func TestInstallTTSServeWarnsOnLowRAM(t *testing.T) {
	e, _ := ttsServeTestEffects(t)
	e.AvailableMemoryMB = func() (int, error) { return 2048, nil }
	var out strings.Builder
	if err := InstallWithOptions(context.Background(), &out, *e, InstallOptions{TTSServe: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "warning: 2048 MB free RAM") {
		t.Fatalf("missing low-RAM warning: %s", out.String())
	}
}

func TestInstallTTSServePinsMatchSpec(t *testing.T) {
	ttsSpec, err := spec.LoadTTS()
	if err != nil {
		t.Fatal(err)
	}
	pins := ttsSpec.TTSServeInstall
	if pins.TTSServeRepo != "https://github.com/scorbo2/tts-serve" {
		t.Fatalf("tts_serve_repo = %q", pins.TTSServeRepo)
	}
	if pins.TTSServeCommit != "6ca92b20e22bfb30eee61e1d96f731ff6a4da4aa" {
		t.Fatalf("tts_serve_commit = %q", pins.TTSServeCommit)
	}
	if pins.ChatterboxRepo != "https://github.com/resemble-ai/chatterbox" {
		t.Fatalf("chatterbox_repo = %q", pins.ChatterboxRepo)
	}
	if pins.ChatterboxCommit != "5de7a54aa4e5e2baadb0182dde554908b48b85c2" {
		t.Fatalf("chatterbox_commit = %q", pins.ChatterboxCommit)
	}
	if pins.MinFreeMemoryMB != 8192 {
		t.Fatalf("min_free_memory_mb = %d, want 8192", pins.MinFreeMemoryMB)
	}
}
