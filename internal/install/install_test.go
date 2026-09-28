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
	if !strings.Contains(help, "--modifierd") || !strings.Contains(help, "--no-tts") || !strings.Contains(help, "--voxcpm") || !strings.Contains(help, "asks before using sudo") {
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
