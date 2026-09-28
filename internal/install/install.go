// Package install implements the safe, user-scoped `voxi install` workflow.
package install

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/spec"
)

// Effects contains host operations used by Install. Tests can inject every
// operation that changes or interrogates the host.
type Effects struct {
	GOOS              string
	GOARCH            string
	Home              string
	Executable        func() (string, error)
	BuildModifier     func(context.Context, string) (string, error)
	LookPath          func(string) (string, error)
	MkdirAll          func(string, os.FileMode) error
	ReadFile          func(string) ([]byte, error)
	Stat              func(string) (os.FileInfo, error)
	WriteFile         func(string, []byte, os.FileMode) error
	Chmod             func(string, os.FileMode) error
	Run               func(context.Context, string, ...string) error
	RunInteractive    func(context.Context, string, ...string) error
	Confirm           func(string) (bool, error)
	RunOutput         func(context.Context, string, ...string) (string, error)
	RunStdin          func(context.Context, string, string, ...string) error
	Symlink           func(string, string) error
	Remove            func(string) error
	DownloadHTTP      func(context.Context, string, string) error
	AvailableMemoryMB func() (int, error)
}

// DefaultEffects binds Install to the current Linux host.
func DefaultEffects() Effects {
	return Effects{
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Home:       os.Getenv("HOME"),
		Executable: os.Executable,
		BuildModifier: func(ctx context.Context, dir string) (string, error) {
			executable, err := os.Executable()
			if err == nil {
				sibling := filepath.Join(filepath.Dir(executable), "voxi-modifierd")
				if _, statErr := os.Stat(sibling); statErr == nil {
					return sibling, nil
				}
			}
			return buildModifier(ctx, dir)
		},
		LookPath:  exec.LookPath,
		MkdirAll:  os.MkdirAll,
		ReadFile:  os.ReadFile,
		Stat:      os.Stat,
		WriteFile: os.WriteFile,
		Chmod:     os.Chmod,
		Run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
		RunInteractive: func(ctx context.Context, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		},
		Confirm: confirmOnTTY,
		RunOutput: func(ctx context.Context, name string, args ...string) (string, error) {
			out, err := exec.CommandContext(ctx, name, args...).Output()
			return string(out), err
		},
		RunStdin: func(ctx context.Context, stdin, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdin = strings.NewReader(stdin)
			return cmd.Run()
		},
		Symlink:           os.Symlink,
		Remove:            os.Remove,
		DownloadHTTP:      downloadResilientHTTP,
		AvailableMemoryMB: availableMemoryMB,
	}
}

// availableMemoryMB reads MemAvailable from /proc/meminfo (Linux only).
func availableMemoryMB() (int, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemAvailable:" {
			kb, err := strconv.Atoi(fields[1])
			if err != nil {
				return 0, fmt.Errorf("parse MemAvailable: %w", err)
			}
			return kb / 1024, nil
		}
	}
	return 0, fmt.Errorf("MemAvailable not found in /proc/meminfo")
}

func confirmOnTTY(prompt string) (bool, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false, nil
	}
	defer tty.Close()
	if _, err := fmt.Fprintf(tty, "%s [Y/n] ", prompt); err != nil {
		return false, err
	}
	answer, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	if err == io.EOF && strings.TrimSpace(answer) == "" {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// InstallOptions controls optional installer behavior without weakening the
// separate privileged --modifierd boundary.
type InstallOptions struct {
	Modifierd  bool
	DisableTTS bool
	TTSServe   bool
}

// Install performs the user installation and, when requested, the privileged
// modifier daemon installation. It stops at the first failed phase.
func Install(ctx context.Context, out io.Writer, e Effects, modifierd bool) error {
	return InstallWithOptions(ctx, out, e, InstallOptions{Modifierd: modifierd})
}

// InstallWithOptions performs the user installation with explicit feature options.
func InstallWithOptions(ctx context.Context, out io.Writer, e Effects, options InstallOptions) error {
	if e.GOOS != "linux" {
		return fmt.Errorf("install: unsupported platform %q (Voxi install requires Linux)", e.GOOS)
	}
	if e.Home == "" {
		return fmt.Errorf("install: HOME is empty; set HOME to a user home directory")
	}
	if e.Executable == nil || e.BuildModifier == nil || e.LookPath == nil || e.MkdirAll == nil || e.ReadFile == nil || e.Stat == nil || e.WriteFile == nil || e.Chmod == nil || e.Run == nil || e.RunInteractive == nil || e.Confirm == nil || e.RunOutput == nil || e.RunStdin == nil || e.Symlink == nil || e.Remove == nil || e.DownloadHTTP == nil || e.AvailableMemoryMB == nil {
		return fmt.Errorf("install: incomplete host effects")
	}
	userBin := filepath.Join(e.Home, ".local", "bin")
	serviceDir := filepath.Join(e.Home, ".config", "systemd", "user")
	voxiPath := filepath.Join(userBin, "voxi")
	settings, err := config.LoadUserSettings(e.Home)
	if err != nil {
		return fmt.Errorf("install: load user settings: %w", err)
	}
	if options.DisableTTS {
		if err := config.SetTTSEnabled(e.Home, false); err != nil {
			return fmt.Errorf("install: save TTS opt-out: %w", err)
		}
		settings.TTSEnabled = false
	}
	models, err := spec.LoadModels()
	if err != nil {
		return fmt.Errorf("install: load model spec: %w", err)
	}
	r2t2Model, modelExists := models.Models["r2t2-confucius4"]
	if !modelExists {
		return fmt.Errorf("install: model spec is missing r2t2-confucius4")
	}
	r2t2Port, validR2T2URL := LoopbackEndpointPort(r2t2Model.BaseURL)
	if !validR2T2URL {
		return fmt.Errorf("install: r2t2-confucius4 base_url %q must use http://127.0.0.1:<port>", r2t2Model.BaseURL)
	}
	r2t2Active := IsR2T2Active(settings.ASRModel, models)
	if r2t2Active {
		for _, name := range []string{"Confucius4-R2T2-Q4_K_M.gguf", "mmproj-Confucius4-R2T2-Q8_0.gguf"} {
			path := filepath.Join(e.Home, ".cache", "voxi", "models", name)
			info, err := e.Stat(path)
			if err != nil {
				return fmt.Errorf("install: R2T2 is selected but model file is unavailable at %s: %w", path, err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("install: R2T2 model path is not a regular file: %s", path)
			}
		}
	}
	llamaServerPath, llamaErr := resolveLlamaServerPath(e, settings.LlamaServerPath)
	if llamaErr != nil && r2t2Active {
		return fmt.Errorf("install: R2T2 is selected but llama-server could not be resolved; set llama_server_path in ~/.config/voxi/config.yaml or install llama-server on PATH: %w", llamaErr)
	}
	if llamaErr != nil {
		// The unit is installed even when disabled. Keep a valid absolute
		// ExecStart so systemd can load it; activation will remain disabled.
		llamaServerPath = "/usr/bin/llama-server"
		fmt.Fprintf(out, "[R2T2 service] unresolved llama-server path; disabled unit uses fallback %s\n", llamaServerPath)
	}

	phase := func(name string, fn func() error) error {
		fmt.Fprintf(out, "[%s] ", name)
		if err := fn(); err != nil {
			fmt.Fprintf(out, "failed: %v\n", err)
			return fmt.Errorf("install phase %s: %w", name, err)
		}
		fmt.Fprintln(out, "ok")
		return nil
	}

	if err := phase("user binary", func() error {
		source, err := e.Executable()
		if err != nil {
			return fmt.Errorf("locate running executable: %w", err)
		}
		data, err := e.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read %s: %w", source, err)
		}
		if err := e.MkdirAll(userBin, 0755); err != nil {
			return fmt.Errorf("create %s: %w", userBin, err)
		}
		_ = e.Remove(voxiPath)
		if err := e.WriteFile(voxiPath, data, 0755); err != nil {
			return fmt.Errorf("write %s: %w", voxiPath, err)
		}
		if err := e.Chmod(voxiPath, 0755); err != nil {
			return err
		}
		goBin := filepath.Join(e.Home, "go", "bin")
		if err := e.MkdirAll(goBin, 0755); err != nil {
			return fmt.Errorf("create %s: %w", goBin, err)
		}
		goBinVoxi := filepath.Join(goBin, "voxi")
		_ = e.Remove(goBinVoxi)
		return e.Symlink(voxiPath, goBinVoxi)
	}); err != nil {
		return err
	}

	if err := phase("user dependencies (crispasr, piper, dotool, dotoold)", func() error {
		return installUserDependencies(ctx, e, userBin, serviceDir)
	}); err != nil {
		return err
	}
	if settings.TTSEnabled {
		if err := phase("TTS system packages", func() error { return installTTSDependencies(ctx, out, e) }); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(out, "[TTS] disabled by config (tts_enabled: false); system packages not changed")
	}

	if err := phase("user service units", func() error {
		if err := e.MkdirAll(serviceDir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", serviceDir, err)
		}
		for _, name := range []string{"voxi-agent.service", "voxi-eager.service", "dotoold.service", "voxi-r2t2.service"} {
			data, err := voxi.ServiceAsset(name)
			if err != nil {
				return fmt.Errorf("read embedded %s: %w", name, err)
			}
			if name == "voxi-agent.service" || name == "voxi-eager.service" {
				data = bytes.ReplaceAll(data, []byte("%h/go/bin/voxi"), []byte("%h/.local/bin/voxi"))
				data = bytes.ReplaceAll(data, []byte("PATH=%h/go/bin:"), []byte("PATH=%h/.local/bin:%h/go/bin:"))
			}
			if name == "dotoold.service" {
				// installUserDependencies writes the dotoold/dotoolc wrapper
				// scripts into userBin (~/.local/bin), not GOBIN (~/go/bin)
				// where `go install` puts the compiled dotool binary. Without
				// this rewrite ExecStart still points at the GOBIN path, so
				// the unit crash-loops forever (203/EXEC) and TypeText silently
				// falls back to a fresh standalone `dotool` process per chunk,
				// each spinning up a new uinput device that drops the first
				// keystrokes while the compositor registers it.
				data = bytes.ReplaceAll(data, []byte("ExecStart=%h/go/bin/dotoold"), []byte("ExecStart=%h/.local/bin/dotoold"))
				// The Makefile's install-dotoold target resolves this via
				// `sed` from `localectl status`; this path must do the same
				// substitution or dotool/xkbcommon fails to compile a keymap
				// for the literal "@DOTOOL_XKB_LAYOUT@" placeholder and the
				// unit exits 1 on every start.
				layout := detectXKBLayout(ctx, e)
				data = bytes.ReplaceAll(data, []byte("@DOTOOL_XKB_LAYOUT@"), []byte(layout))
			}
			if name == "voxi-r2t2.service" {
				data = bytes.ReplaceAll(data, []byte("@LLAMA_SERVER_PATH@"), []byte(llamaServerPath))
				data = bytes.ReplaceAll(data, []byte("@R2T2_PORT@"), []byte(r2t2Port))
			}
			if err := e.WriteFile(filepath.Join(serviceDir, name), data, 0644); err != nil {
				return fmt.Errorf("write %s: %w", name, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if err := phase("user service activation", func() error {
		if err := e.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
			return fmt.Errorf("reload user manager: %w", err)
		}
		if err := e.Run(ctx, "systemctl", "--user", "enable", "--now", "dotoold.service"); err != nil {
			return fmt.Errorf("enable/start dotoold.service: %w", err)
		}
		if err := e.Run(ctx, "systemctl", "--user", "enable", "--now", "voxi-agent.service"); err != nil {
			return fmt.Errorf("enable/start voxi-agent.service: %w", err)
		}
		if r2t2Active {
			if err := e.Run(ctx, "systemctl", "--user", "enable", "--now", "voxi-r2t2.service"); err != nil {
				return fmt.Errorf("enable/start voxi-r2t2.service: %w", err)
			}
		} else {
			if err := e.Run(ctx, "systemctl", "--user", "disable", "--now", "voxi-r2t2.service"); err != nil {
				return fmt.Errorf("disable/stop voxi-r2t2.service: %w", err)
			}
			fmt.Fprintf(out, "[R2T2 service] disabled (selected ASR model %q does not use a loopback openai-transcribe endpoint)\n", settings.ASRModel)
		}
		return nil
	}); err != nil {
		return err
	}

	if options.TTSServe {
		if err := phase("tts-serve (Chatterbox)", func() error {
			return installTTSServe(ctx, out, e)
		}); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(out, "[tts-serve] skipped (use --tts-serve to set up the Chatterbox tts-serve server)")
	}

	if !options.Modifierd {
		fmt.Fprintln(out, "[modifier daemon] skipped (use --modifierd to request privileged system setup)")
		return nil
	}

	return phase("modifier daemon (privileged)", func() error {
		buildDir := filepath.Join(e.Home, ".cache", "voxi", "install")
		if err := e.MkdirAll(buildDir, 0700); err != nil {
			return fmt.Errorf("create build directory: %w", err)
		}
		binary, err := e.BuildModifier(ctx, buildDir)
		if err != nil {
			return fmt.Errorf("build voxi-modifierd: %w", err)
		}
		modifierService, err := voxi.ServiceAsset("voxi-modifierd.service")
		if err != nil {
			return fmt.Errorf("read embedded voxi-modifierd.service: %w", err)
		}
		serviceSource := filepath.Join(buildDir, "voxi-modifierd.service")
		if err := e.WriteFile(serviceSource, modifierService, 0644); err != nil {
			return fmt.Errorf("stage modifier service: %w", err)
		}
		if err := e.Run(ctx, "sudo", "install", "-m", "0755", binary, "/usr/local/bin/voxi-modifierd"); err != nil {
			return fmt.Errorf("sudo install modifier binary: %w", err)
		}
		if err := e.Run(ctx, "sudo", "install", "-m", "0644", serviceSource, "/etc/systemd/system/voxi-modifierd.service"); err != nil {
			return fmt.Errorf("sudo install modifier service: %w", err)
		}
		for _, args := range [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "voxi-modifierd.service"},
		} {
			if err := e.Run(ctx, "sudo", args...); err != nil {
				return fmt.Errorf("sudo %s: %w", args[0], err)
			}
		}
		return nil
	})
}

// LoopbackEndpointPort parses rawURL and returns the port number if rawURL is an http://127.0.0.1:<port> URL.
func LoopbackEndpointPort(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" {
		return "", false
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", false
	}
	return strconv.Itoa(port), true
}

// SameLoopbackEndpoint returns true if candidate points to the same loopback host and port as target.
func SameLoopbackEndpoint(candidate, target string) bool {
	if _, ok := LoopbackEndpointPort(target); !ok {
		return false
	}
	targetURL, err := url.Parse(target)
	if err != nil {
		return false
	}
	candidateURL, err := url.Parse(candidate)
	if err != nil || candidateURL.Scheme != "http" {
		return false
	}
	return candidateURL.Hostname() == targetURL.Hostname() && candidateURL.Port() == targetURL.Port()
}

// IsR2T2Active returns true if selectedModelName uses the local R2T2 backend endpoint.
func IsR2T2Active(selectedModelName string, models *spec.ModelSpec) bool {
	if models == nil {
		return false
	}
	r2t2Model, modelExists := models.Models["r2t2-confucius4"]
	if !modelExists {
		return false
	}
	selectedModel, selectedExists := models.Models[selectedModelName]
	return selectedExists && selectedModel.Engine == "openai-transcribe" && SameLoopbackEndpoint(selectedModel.BaseURL, r2t2Model.BaseURL)
}

func resolveLlamaServerPath(e Effects, configured string) (string, error) {
	name := configured
	if name == "" {
		name = "llama-server"
	}
	path, err := e.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("look up %q: %w", name, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make llama-server path absolute: %w", err)
	}
	return absolute, nil
}

// detectXKBLayout mirrors the Makefile install-dotoold target's
// `localectl status` probe. Falls back to "us" when localectl is
// unavailable or reports no X11 layout, since dotool refuses to run
// without a resolvable xkb_symbols layout.
func detectXKBLayout(ctx context.Context, e Effects) string {
	out, err := e.RunOutput(ctx, "localectl", "status")
	if err != nil {
		return "us"
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "X11 Layout:"); ok {
			if layout := strings.TrimSpace(rest); layout != "" {
				return layout
			}
		}
	}
	return "us"
}

func installUserDependencies(ctx context.Context, e Effects, userBin, serviceDir string) error {
	if e.GOARCH != "amd64" && e.GOARCH != "arm64" {
		return fmt.Errorf("unsupported architecture %q for CrispASR prebuilt release", e.GOARCH)
	}
	asset := "crispasr-linux-x86_64.tar.gz"
	if e.GOARCH == "arm64" {
		asset = "crispasr-linux-arm64.tar.gz"
	}
	crispDir := filepath.Join(e.Home, ".local", "lib", "voxi", "crispasr")
	if err := e.MkdirAll(crispDir, 0755); err != nil {
		return fmt.Errorf("create CrispASR directory: %w", err)
	}
	archive := filepath.Join(e.Home, ".cache", "voxi", asset)
	if err := e.MkdirAll(filepath.Dir(archive), 0700); err != nil {
		return fmt.Errorf("create download directory: %w", err)
	}
	url := "https://github.com/CrispStrobe/CrispASR/releases/latest/download/" + asset
	if err := downloadCrispASR(ctx, e, archive, url); err != nil {
		return err
	}
	if err := e.Run(ctx, "tar", "-xzf", archive, "-C", crispDir, "--strip-components=1"); err != nil {
		return fmt.Errorf("extract CrispASR: %w", err)
	}
	crispBin := filepath.Join(userBin, "crispasr")
	if err := e.Remove(crispBin); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace crispasr link: %w", err)
	}
	if err := e.Symlink(filepath.Join(crispDir, "crispasr"), crispBin); err != nil {
		return fmt.Errorf("link crispasr: %w", err)
	}
	if err := e.Run(ctx, crispBin, "--version"); err != nil {
		return fmt.Errorf("verify crispasr: %w", err)
	}
	if err := installPiper(ctx, e, userBin); err != nil {
		return err
	}
	if err := e.Run(ctx, "go", "install", "git.sr.ht/~geb/dotool@latest"); err != nil {
		return fmt.Errorf("install dotool: %w", err)
	}
	moduleDir, err := e.RunOutput(ctx, "go", "list", "-m", "-f", "{{.Dir}}", "git.sr.ht/~geb/dotool@latest")
	if err != nil {
		return fmt.Errorf("locate dotool package: %w", err)
	}
	for _, name := range []string{"dotoold", "dotoolc"} {
		data, err := e.ReadFile(filepath.Join(strings.TrimSpace(moduleDir), name))
		if err != nil {
			return fmt.Errorf("read dotool %s: %w", name, err)
		}
		path := filepath.Join(userBin, name)
		if err := e.WriteFile(path, data, 0755); err != nil {
			return fmt.Errorf("install dotool %s: %w", name, err)
		}
		if err := e.Chmod(path, 0755); err != nil {
			return fmt.Errorf("make dotool %s executable: %w", name, err)
		}
	}
	_ = serviceDir // unit installation is kept in the following phase.
	return nil
}

func installPiper(ctx context.Context, e Effects, userBin string) error {
	if e.GOARCH != "amd64" && e.GOARCH != "arm64" {
		return fmt.Errorf("unsupported architecture %q for Piper prebuilt release", e.GOARCH)
	}
	asset := "piper_linux_x86_64.tar.gz"
	if e.GOARCH == "arm64" {
		asset = "piper_linux_aarch64.tar.gz"
	}
	piperDir := filepath.Join(e.Home, ".local", "lib", "voxi", "piper")
	if err := e.MkdirAll(piperDir, 0755); err != nil {
		return fmt.Errorf("create Piper directory: %w", err)
	}
	archive := filepath.Join(e.Home, ".cache", "voxi", asset)
	if err := e.MkdirAll(filepath.Dir(archive), 0700); err != nil {
		return fmt.Errorf("create download directory: %w", err)
	}
	url := "https://github.com/rhasspy/piper/releases/latest/download/" + asset
	if err := downloadPiper(ctx, e, archive, url); err != nil {
		return err
	}
	if err := e.Run(ctx, "tar", "-xzf", archive, "-C", piperDir, "--strip-components=1"); err != nil {
		return fmt.Errorf("extract Piper: %w", err)
	}
	piperBin := filepath.Join(userBin, "piper")
	if err := e.Remove(piperBin); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace Piper link: %w", err)
	}
	if err := e.Symlink(filepath.Join(piperDir, "piper"), piperBin); err != nil {
		return fmt.Errorf("link Piper: %w", err)
	}
	voices := filepath.Join(e.Home, ".local", "share", "voxi", "voices")
	if err := e.MkdirAll(voices, 0755); err != nil {
		return fmt.Errorf("create Piper voices directory: %w", err)
	}
	for _, suffix := range []string{"onnx", "onnx.json"} {
		name := "en_US-lessac-medium." + suffix
		voiceURL := "https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_US/lessac/medium/" + name
		target := filepath.Join(voices, name)
		if err := downloadPiperVoice(ctx, e, target, voiceURL); err != nil {
			return fmt.Errorf("download Piper voice %s: %w", name, err)
		}
	}
	return nil
}

func downloadPiper(ctx context.Context, e Effects, archive, url string) error {
	if _, err := os.Stat(archive); err == nil {
		if err := e.Run(ctx, "tar", "-tzf", archive); err == nil {
			return nil
		}
	}

	curlErr := e.Run(ctx, "curl", "-fL", "-o", archive, url)
	if curlErr == nil {
		return nil
	}

	dlErr := e.DownloadHTTP(ctx, url, archive)
	if dlErr == nil {
		return nil
	}

	var hints []string
	if curlErr != nil {
		hints = append(hints, fmt.Sprintf("curl error: %v", curlErr))
	}
	if dlErr != nil {
		hints = append(hints, fmt.Sprintf("HTTP fallback error: %v", dlErr))
	}
	hintMsg := ""
	if len(hints) > 0 {
		hintMsg = "\n\nDiagnostic details:\n  - " + strings.Join(hints, "\n  - ")
	}

	return fmt.Errorf("download Piper failed%s\n\n"+
		"To resolve manually:\n"+
		"  1. Download: %s\n"+
		"  2. Place at: %s\n"+
		"  3. Re-run: 'voxi install'",
		hintMsg, url, archive)
}

func downloadPiperVoice(ctx context.Context, e Effects, target, url string) error {
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		return nil
	}
	if err := e.Run(ctx, "curl", "-fL", "-o", target, url); err == nil {
		return nil
	}
	if err := e.DownloadHTTP(ctx, url, target); err != nil {
		return fmt.Errorf("curl and HTTP download failed: %w", err)
	}
	return nil
}

func installTTSDependencies(ctx context.Context, out io.Writer, e Effects) error {
	missing := missingTTSTools(e.LookPath)
	if len(missing) == 0 {
		fmt.Fprintln(out, "TTS engine, fallback, and audio player already available")
		return nil
	}
	manager := ""
	packages := []string{}
	if _, err := e.LookPath("dnf"); err == nil {
		manager = "dnf"
		packages = []string{"festival", "espeak-ng", "pipewire-utils"}
	} else if _, err := e.LookPath("apt-get"); err == nil {
		manager = "apt-get"
		packages = []string{"festival", "espeak-ng", "pipewire-bin"}
	}
	if manager == "" {
		fmt.Fprintf(out, "TTS tools missing (%s); install Festival, espeak-ng, and pw-play/paplay with your package manager\n", strings.Join(missing, ", "))
		return nil
	}
	confirmed, err := e.Confirm(fmt.Sprintf("Install TTS dependencies (%s) with sudo %s?", strings.Join(packages, ", "), manager))
	if err != nil {
		fmt.Fprintf(out, "TTS package installation skipped because confirmation failed: %v\n", err)
		return nil
	}
	if !confirmed {
		fmt.Fprintf(out, "TTS package installation skipped; missing: %s\n", strings.Join(missing, ", "))
		return nil
	}
	args := append([]string{manager, "install"}, packages...)
	if err := e.RunInteractive(ctx, "sudo", args...); err != nil {
		return fmt.Errorf("install TTS packages with sudo %s: %w", manager, err)
	}
	if remaining := missingTTSTools(e.LookPath); len(remaining) != 0 {
		return fmt.Errorf("TTS package installation completed but required tools are still missing: %s", strings.Join(remaining, ", "))
	}
	fmt.Fprintln(out, "TTS engine, fallback, and audio player installed")
	return nil
}

// installTTSServe sets up a restart-safe Chatterbox tts-serve server (issue
// 159 M1): a pinned tts-serve checkout and Python venv under
// ~/.cache/voxi/tts-serve, and a voxi-tts-serve.service user unit enabled
// against the port from spec/tts.yaml's tts_serve.url. Declining the
// confirmation prompt leaves the host untouched.
func installTTSServe(ctx context.Context, out io.Writer, e Effects) error {
	ttsSpec, err := spec.LoadTTS()
	if err != nil {
		return fmt.Errorf("load TTS spec: %w", err)
	}
	port, validURL := LoopbackEndpointPort(ttsSpec.TTSServe.URL)
	if !validURL {
		return fmt.Errorf("tts_serve.url %q must use http://127.0.0.1:<port>", ttsSpec.TTSServe.URL)
	}
	pins := ttsSpec.TTSServeInstall

	if memMB, memErr := e.AvailableMemoryMB(); memErr == nil && memMB > 0 && memMB < pins.MinFreeMemoryMB {
		fmt.Fprintf(out, "\n[tts-serve] warning: %d MB free RAM available; Chatterbox on CPU wants about %d MB\n", memMB, pins.MinFreeMemoryMB)
	}

	prompt := fmt.Sprintf(
		"Set up Chatterbox tts-serve under ~/.cache/voxi/tts-serve (clones %s @ %s, pip-installs PyTorch and %s @ %s into a venv; a multi-GB download)?",
		pins.TTSServeRepo, pins.TTSServeCommit, pins.ChatterboxRepo, pins.ChatterboxCommit,
	)
	confirmed, err := e.Confirm(prompt)
	if err != nil {
		return fmt.Errorf("tts-serve confirmation: %w", err)
	}
	if !confirmed {
		fmt.Fprintln(out, "tts-serve setup declined; nothing installed")
		return nil
	}

	dir := filepath.Join(e.Home, ".cache", "voxi", "tts-serve")
	if err := e.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dir), err)
	}
	if _, err := e.Stat(filepath.Join(dir, ".git")); err != nil {
		if err := e.Run(ctx, "git", "clone", pins.TTSServeRepo, dir); err != nil {
			return fmt.Errorf("clone tts-serve: %w", err)
		}
	}
	if err := e.Run(ctx, "git", "-C", dir, "fetch", "origin", pins.TTSServeCommit); err != nil {
		return fmt.Errorf("fetch tts-serve commit %s: %w", pins.TTSServeCommit, err)
	}
	if err := e.Run(ctx, "git", "-C", dir, "checkout", pins.TTSServeCommit); err != nil {
		return fmt.Errorf("checkout tts-serve commit %s: %w", pins.TTSServeCommit, err)
	}

	venv := filepath.Join(dir, ".venv")
	venvPython := filepath.Join(venv, "bin", "python")
	if _, err := e.Stat(venvPython); err != nil {
		if err := e.Run(ctx, "python3", "-m", "venv", venv); err != nil {
			return fmt.Errorf("create tts-serve venv: %w", err)
		}
	}
	pip := filepath.Join(venv, "bin", "pip")
	if err := e.Run(ctx, pip, "install", fmt.Sprintf("git+%s@%s", pins.ChatterboxRepo, pins.ChatterboxCommit)); err != nil {
		return fmt.Errorf("pip install chatterbox: %w", err)
	}
	if err := e.Run(ctx, pip, "install", filepath.Join(dir, "tts-engine-common"), "fastapi", "uvicorn", "loguru", "soundfile"); err != nil {
		return fmt.Errorf("pip install tts-serve dependencies: %w", err)
	}

	serviceDir := filepath.Join(e.Home, ".config", "systemd", "user")
	if err := e.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", serviceDir, err)
	}
	data, err := voxi.ServiceAsset("voxi-tts-serve.service")
	if err != nil {
		return fmt.Errorf("read embedded voxi-tts-serve.service: %w", err)
	}
	data = bytes.ReplaceAll(data, []byte("@TTS_SERVE_PORT@"), []byte(port))
	if err := e.WriteFile(filepath.Join(serviceDir, "voxi-tts-serve.service"), data, 0644); err != nil {
		return fmt.Errorf("write voxi-tts-serve.service: %w", err)
	}
	if err := e.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("reload user manager: %w", err)
	}
	if err := e.Run(ctx, "systemctl", "--user", "enable", "--now", "voxi-tts-serve.service"); err != nil {
		return fmt.Errorf("enable/start voxi-tts-serve.service: %w", err)
	}
	return nil
}

func missingTTSTools(lookPath func(string) (string, error)) []string {
	var missing []string
	if _, err := lookPath("text2wave"); err != nil {
		missing = append(missing, "text2wave (Festival)")
	}
	if _, err := lookPath("espeak-ng"); err != nil {
		missing = append(missing, "espeak-ng")
	}
	if _, err := lookPath("pw-play"); err != nil {
		if _, paplayErr := lookPath("paplay"); paplayErr != nil {
			missing = append(missing, "pw-play or paplay")
		}
	}
	return missing
}

func downloadCrispASR(ctx context.Context, e Effects, archive, url string) error {
	// If the archive already exists on disk and is a valid tarball, reuse it.
	if _, err := os.Stat(archive); err == nil {
		if err := e.Run(ctx, "tar", "-tzf", archive); err == nil {
			return nil
		}
	}

	// Step 1: Try curl with resume and retry
	curlErr := e.Run(ctx, "curl", "-fL", "-o", archive, url)
	if curlErr == nil {
		return nil
	}

	// Step 2: If curl fails, try resilient Go HTTP range downloader
	dlErr := e.DownloadHTTP(ctx, url, archive)
	if dlErr == nil {
		return nil
	}

	// Step 3: Format clear, actionable diagnostics
	var hints []string
	if curlErr != nil {
		if strings.Contains(curlErr.Error(), "exit status 56") {
			hints = append(hints, "curl reported exit status 56 (network/TLS receive failure, often caused by MTU mismatch or packet drops on Wi-Fi/VPN)")
		} else {
			hints = append(hints, fmt.Sprintf("curl error: %v", curlErr))
		}
	}
	if dlErr != nil {
		hints = append(hints, fmt.Sprintf("HTTP range fallback error: %v", dlErr))
	}

	hintMsg := ""
	if len(hints) > 0 {
		hintMsg = "\n\nDiagnostic details:\n  - " + strings.Join(hints, "\n  - ")
	}

	return fmt.Errorf("download CrispASR failed%s\n\n"+
		"To resolve manually:\n"+
		"  1. Download: %s\n"+
		"  2. Place at: %s\n"+
		"  3. Re-run: 'voxi install'",
		hintMsg, url, archive)
}

func downloadResilientHTTP(ctx context.Context, url, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	finalURL := resp.Request.URL.String()
	totalSize := resp.ContentLength

	tmpFile := targetPath + ".tmp"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(tmpFile)
	}()

	if totalSize > 0 && (resp.Header.Get("Accept-Ranges") == "bytes" || resp.StatusCode == 200) {
		chunkSize := int64(256 * 1024) // 256 KB chunks
		for start := int64(0); start < totalSize; {
			end := start + chunkSize - 1
			if end >= totalSize {
				end = totalSize - 1
			}
			var chunkData []byte
			var chunkErr error
			for attempt := 0; attempt < 5; attempt++ {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				chunkReq, err := http.NewRequestWithContext(ctx, "GET", finalURL, nil)
				if err != nil {
					chunkErr = err
					continue
				}
				chunkReq.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
				chunkReq.Close = true // close connection per chunk to avoid TLS record framing corruption

				cClient := &http.Client{Timeout: 15 * time.Second}
				cResp, err := cClient.Do(chunkReq)
				if err != nil {
					chunkErr = err
					time.Sleep(100 * time.Millisecond)
					continue
				}
				data, err := io.ReadAll(cResp.Body)
				cResp.Body.Close()
				if err != nil {
					chunkErr = err
					time.Sleep(100 * time.Millisecond)
					continue
				}
				if int64(len(data)) != (end - start + 1) {
					chunkErr = fmt.Errorf("short read: got %d bytes, want %d", len(data), end-start+1)
					time.Sleep(100 * time.Millisecond)
					continue
				}
				chunkData = data
				chunkErr = nil
				break
			}
			if chunkErr != nil {
				return fmt.Errorf("download chunk %d-%d: %w", start, end, chunkErr)
			}
			if _, err := f.Write(chunkData); err != nil {
				return err
			}
			start = end + 1
		}
	} else {
		getReq, err := http.NewRequestWithContext(ctx, "GET", finalURL, nil)
		if err != nil {
			return err
		}
		getResp, err := client.Do(getReq)
		if err != nil {
			return err
		}
		defer getResp.Body.Close()
		if getResp.StatusCode >= 400 {
			return fmt.Errorf("HTTP %d: %s", getResp.StatusCode, getResp.Status)
		}
		if _, err := io.Copy(f, getResp.Body); err != nil {
			return err
		}
	}

	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmpFile, targetPath)
}

func buildModifier(ctx context.Context, dir string) (string, error) {
	path := filepath.Join(dir, "voxi-modifierd")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	// Check if already installed in system or user binary directories
	for _, cand := range []string{
		"/usr/local/bin/voxi-modifierd",
		filepath.Join(os.Getenv("HOME"), "go", "bin", "voxi-modifierd"),
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "voxi-modifierd"),
	} {
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	mod, err := exec.CommandContext(ctx, "go", "env", "GOMOD").Output()
	if err == nil && strings.TrimSpace(string(mod)) != "" && strings.TrimSpace(string(mod)) != "/dev/null" {
		root := filepath.Dir(strings.TrimSpace(string(mod)))
		cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "./cmd/voxi-modifierd")
		cmd.Dir = root
		if err := cmd.Run(); err == nil {
			return path, nil
		}
	}
	// If outside a local source checkout, attempt to build from remote module if Go toolchain is available
	if _, err := exec.LookPath("go"); err == nil {
		cmd := exec.CommandContext(ctx, "go", "install", "ubunatic.com/voxi/cmd/voxi-modifierd@latest")
		cmd.Env = append(os.Environ(), "GOBIN="+dir)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return path, nil
		}
		return "", fmt.Errorf("install voxi-modifierd from remote module: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return "", fmt.Errorf("no source checkout or packaged voxi-modifierd binary available (install Go to build or place voxi-modifierd in PATH)")
}

// NewCommand returns the Cobra command used by the main CLI.
func NewCommand(e Effects, out io.Writer) *cobra.Command {
	var modifierd bool
	var noTTS bool
	var ttsServe bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Voxi for this user (optional: --modifierd needs sudo)",
		Long: "Install the CLI and systemd user services under your home directory, then enable and start voxi-agent.service.\n" +
			"When TTS tools are missing, Voxi asks before using sudo and the system package manager.\n\n" +
			"Use --no-tts to persistently opt out of TTS packages. Use --modifierd for the optional system-wide physical modifier daemon; it also requires sudo and Linux systemd.\n" +
			"Use --tts-serve to set up and enable a Chatterbox tts-serve server (multi-GB download, asks first).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return InstallWithOptions(cmd.Context(), out, e, InstallOptions{Modifierd: modifierd, DisableTTS: noTTS, TTSServe: ttsServe})
		},
	}
	cmd.Flags().BoolVar(&modifierd, "modifierd", false, "also install the optional system-wide modifier daemon (requires sudo)")
	cmd.Flags().BoolVar(&noTTS, "no-tts", false, "disable TTS and skip Festival/espeak-ng system packages")
	cmd.Flags().BoolVar(&ttsServe, "tts-serve", false, "set up and enable a Chatterbox tts-serve server (multi-GB download, asks first)")
	return cmd
}
