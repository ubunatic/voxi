package tts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

func TestEngineBackendStatusReportsPreferredFallbackAndMissing(t *testing.T) {
	for _, tc := range []struct {
		name      string
		available map[string]bool
		want      string
	}{
		{name: "preferred", available: map[string]bool{"text2wave": true, "espeak-ng": true, "pw-play": true}, want: "Festival; player pw-play"},
		{name: "fallback", available: map[string]bool{"espeak-ng": true, "paplay": true}, want: "espeak-ng fallback; player paplay"},
		{name: "missing", available: map[string]bool{}, want: "missing (Festival / espeak-ng); player missing (pw-play / paplay)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			engine := NewEngine(deps.Dependencies{
				Getenv: func(key string) string {
					if key == "HOME" {
						return home
					}
					return ""
				},
				LookPath: func(name string) (string, error) {
					if tc.available[name] {
						return "/usr/bin/" + name, nil
					}
					return "", os.ErrNotExist
				},
			}, "/usr/bin/voxi")
			if got := engine.BackendStatus(); got != tc.want {
				t.Fatalf("BackendStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEngineBackendStatusReportsConfiguredPiperAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		available map[string]bool
		want      string
	}{
		{name: "piper selected", available: map[string]bool{"piper": true, "pw-play": true}, want: "Piper; player pw-play"},
		{name: "piper missing falls back", available: map[string]bool{"espeak-ng": true, "paplay": true}, want: "espeak-ng fallback (Piper unavailable: piper executable missing); player paplay"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := filepath.Join(t.TempDir(), "voice.onnx")
			if err := os.WriteFile(model, []byte("test model placeholder"), 0600); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(deps.Dependencies{
				Getenv: func(key string) string {
					if key == "VOXI_TTS_BACKEND" {
						return "piper"
					}
					if key == "VOXI_PIPER_MODEL" {
						return model
					}
					return ""
				},
				LookPath: func(name string) (string, error) {
					if tc.available[name] {
						return "/usr/bin/" + name, nil
					}
					return "", os.ErrNotExist
				},
			}, "/usr/bin/voxi")
			if got := engine.BackendStatus(); got != tc.want {
				t.Fatalf("BackendStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSelectedBackendUsesAutoByDefaultAndNormalizesValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	backend := "  PiPeR "
	engine := NewEngine(deps.Dependencies{Getenv: func(key string) string {
		if key == "VOXI_TTS_BACKEND" {
			return backend
		}
		return ""
	}}, "/usr/bin/voxi")
	if got := engine.selectedBackend(); got != "piper" {
		t.Fatalf("selectedBackend() = %q, want piper", got)
	}
	engine.deps.Getenv = func(string) string { return "" }
	if got := engine.selectedBackend(); got != "auto" {
		t.Fatalf("selectedBackend() = %q, want auto", got)
	}
}

func TestSynthesizeVoxCPMPlaceholderReturnsIssue160Error(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "voxi")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("tts_backend: voxcpm\n"), 0600); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(deps.Dependencies{Getenv: func(key string) string {
		if key == "HOME" {
			return home
		}
		return ""
	}}, "/usr/bin/voxi")
	_, _, err := engine.Synthesize(t.Context(), "hello")
	if err == nil || !strings.Contains(err.Error(), "not yet implemented") || !strings.Contains(err.Error(), "issue 160") {
		t.Fatalf("Synthesize() error = %v, want placeholder issue 160 error", err)
	}
}

func TestEngineLoadsTTSSettingsAndEnvironmentOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "voxi")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("tts_backend: festival\ntts_piper_model: yaml.onnx\ntts_piper_config: yaml.json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(deps.Dependencies{}, "/usr/bin/voxi")
	if engine.selectedBackend() != "festival" || engine.piperModelPath() != "yaml.onnx" || engine.piperConfigPath() != "yaml.json" {
		t.Fatalf("loaded settings = backend:%q model:%q config:%q", engine.selectedBackend(), engine.piperModelPath(), engine.piperConfigPath())
	}
	engine.deps.Getenv = func(key string) string {
		return map[string]string{"VOXI_TTS_BACKEND": "piper", "VOXI_PIPER_MODEL": "override.onnx", "VOXI_PIPER_CONFIG": "override.json"}[key]
	}
	if engine.selectedBackend() != "piper" || engine.piperModelPath() != "override.onnx" || engine.piperConfigPath() != "override.json" {
		t.Fatalf("overrides = backend:%q model:%q config:%q", engine.selectedBackend(), engine.piperModelPath(), engine.piperConfigPath())
	}
}

func TestNewEngineWithCustomHomeAndMalformedConfig(t *testing.T) {
	tempHome := t.TempDir()
	configDir := filepath.Join(tempHome, ".config", "voxi")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write malformed YAML
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("invalid: [yaml: broken"), 0600); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return tempHome
			}
			return ""
		},
	}, "/usr/bin/voxi")
	if engine.backend != "auto" {
		t.Fatalf("expected fallback backend 'auto' on malformed config, got %q", engine.backend)
	}
}

func TestSynthesizeExplicitPiperFailsWhenExecutableOrModelMissing(t *testing.T) {
	ctx := t.Context()
	// Case 1: Model unset
	engine := NewEngine(deps.Dependencies{
		Getenv: func(key string) string {
			if key == "VOXI_TTS_BACKEND" {
				return "piper"
			}
			return ""
		},
		LookPath: func(name string) (string, error) {
			if name == "piper" {
				return "/usr/bin/piper", nil
			}
			return "", os.ErrNotExist
		},
	}, "/usr/bin/voxi")
	if _, _, err := engine.Synthesize(ctx, "hello"); err == nil || !strings.Contains(err.Error(), "model file") {
		t.Fatalf("expected default model missing error, got: %v", err)
	}

	// Case 2: Piper missing
	engine = NewEngine(deps.Dependencies{
		Getenv: func(key string) string {
			if key == "VOXI_TTS_BACKEND" {
				return "piper"
			}
			if key == "VOXI_PIPER_MODEL" {
				return "/tmp/nonexistent.onnx"
			}
			return ""
		},
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	}, "/usr/bin/voxi")
	if _, _, err := engine.Synthesize(ctx, "hello"); err == nil || !strings.Contains(err.Error(), "piper executable not found") {
		t.Fatalf("expected piper not found error, got: %v", err)
	}
}

func TestSupervisorHelperProcess(t *testing.T) {
	if os.Getenv("VOXI_TTS_SUPERVISOR_TEST_HELPER") != "1" {
		return
	}
	pidFile := os.Getenv("VOXI_TTS_SUPERVISOR_TEST_PID_FILE")
	setsid, err := exec.LookPath("setsid")
	if err != nil {
		t.Skip("setsid is required to exercise descendants in a separate process group")
	}
	command := fmt.Sprintf("%q sleep 90 & echo $! > %q; wait", setsid, pidFile)
	if err := RunSupervisor([]string{"/bin/sh", "-c", command}); err != nil {
		t.Fatalf("RunSupervisor() = %v", err)
	}
}

func TestSupervisorParentDeathKillsAndReapsGrandchild(t *testing.T) {
	if testing.Short() {
		t.Skip("process-tree integration test")
	}
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	owner := exec.Command(executable, "-test.run=^TestSupervisorOwnerHelper$")
	owner.Env = append(os.Environ(), "VOXI_TTS_SUPERVISOR_TEST_OWNER=1", "VOXI_TTS_SUPERVISOR_TEST_PID_FILE="+pidFile)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	grandchildPID := waitPIDFile(t, pidFile)
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = owner.Wait()
	waitProcessExit(t, grandchildPID)
}

func TestSupervisorOwnerHelper(t *testing.T) {
	if os.Getenv("VOXI_TTS_SUPERVISOR_TEST_OWNER") != "1" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor := exec.Command(executable, "-test.run=^TestSupervisorHelperProcess$")
	supervisor.Env = append(os.Environ(), "VOXI_TTS_SUPERVISOR_TEST_HELPER=1")
	supervisor.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	if err := supervisor.Start(); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.Wait(); err != nil {
		t.Fatalf("supervisor exited unexpectedly: %v", err)
	}
}

func waitPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("supervised descendant did not report its PID")
	return 0
}

func waitProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) {
			return
		}
		if err == nil {
			fields := strings.Fields(string(data))
			if len(fields) > 2 && fields[2] == "Z" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("supervised descendant %d survived owner SIGKILL", pid)
}
