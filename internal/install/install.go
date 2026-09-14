// Package install implements the safe, user-scoped `voxi install` workflow.
package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi"
)

// Effects contains host operations used by Install. Tests can inject every
// operation that changes or interrogates the host.
type Effects struct {
	GOOS          string
	GOARCH        string
	Home          string
	Executable    func() (string, error)
	BuildModifier func(context.Context, string) (string, error)
	LookPath      func(string) (string, error)
	MkdirAll      func(string, os.FileMode) error
	ReadFile      func(string) ([]byte, error)
	WriteFile     func(string, []byte, os.FileMode) error
	Chmod         func(string, os.FileMode) error
	Run           func(context.Context, string, ...string) error
	RunOutput     func(context.Context, string, ...string) (string, error)
	RunStdin      func(context.Context, string, string, ...string) error
	Symlink       func(string, string) error
	Remove        func(string) error
	DownloadHTTP  func(context.Context, string, string) error
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
		WriteFile: os.WriteFile,
		Chmod:     os.Chmod,
		Run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
		RunOutput: func(ctx context.Context, name string, args ...string) (string, error) {
			out, err := exec.CommandContext(ctx, name, args...).Output()
			return string(out), err
		},
		RunStdin: func(ctx context.Context, stdin, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdin = strings.NewReader(stdin)
			return cmd.Run()
		},
		Symlink:      os.Symlink,
		Remove:       os.Remove,
		DownloadHTTP: downloadResilientHTTP,
	}
}

// Install performs the user installation and, when requested, the privileged
// modifier daemon installation. It stops at the first failed phase.
func Install(ctx context.Context, out io.Writer, e Effects, modifierd bool) error {
	if e.GOOS != "linux" {
		return fmt.Errorf("install: unsupported platform %q (Voxi install requires Linux)", e.GOOS)
	}
	if e.Home == "" {
		return fmt.Errorf("install: HOME is empty; set HOME to a user home directory")
	}
	if e.Executable == nil || e.BuildModifier == nil || e.LookPath == nil || e.MkdirAll == nil || e.ReadFile == nil || e.WriteFile == nil || e.Chmod == nil || e.Run == nil || e.RunOutput == nil || e.RunStdin == nil || e.Symlink == nil || e.Remove == nil || e.DownloadHTTP == nil {
		return fmt.Errorf("install: incomplete host effects")
	}
	userBin := filepath.Join(e.Home, ".local", "bin")
	serviceDir := filepath.Join(e.Home, ".config", "systemd", "user")
	voxiPath := filepath.Join(userBin, "voxi")

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

	if err := phase("user dependencies (crispasr, dotool, dotoold)", func() error {
		return installUserDependencies(ctx, e, userBin, serviceDir)
	}); err != nil {
		return err
	}

	if err := phase("user service units", func() error {
		if err := e.MkdirAll(serviceDir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", serviceDir, err)
		}
		for _, name := range []string{"voxi-agent.service", "voxi-eager.service", "dotoold.service"} {
			data, err := voxi.ServiceAsset(name)
			if err != nil {
				return fmt.Errorf("read embedded %s: %w", name, err)
			}
			if name == "voxi-agent.service" || name == "voxi-eager.service" {
				data = bytes.ReplaceAll(data, []byte("%h/go/bin/voxi"), []byte("%h/.local/bin/voxi"))
				data = bytes.ReplaceAll(data, []byte("PATH=%h/go/bin:"), []byte("PATH=%h/.local/bin:%h/go/bin:"))
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
		return nil
	}); err != nil {
		return err
	}

	if !modifierd {
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
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Voxi for this user (optional: --modifierd needs sudo)",
		Long: "Install the CLI and systemd user services under your home directory, then enable and start voxi-agent.service.\n" +
			"This default path never invokes sudo or writes system locations.\n\n" +
			"Use --modifierd only to install the optional system-wide physical modifier daemon; it requires sudo and Linux systemd.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return Install(cmd.Context(), out, e, modifierd)
		},
	}
	cmd.Flags().BoolVar(&modifierd, "modifierd", false, "also install the optional system-wide modifier daemon (requires sudo)")
	return cmd
}
