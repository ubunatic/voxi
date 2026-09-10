// Package install implements the safe, user-scoped `voxi install` workflow.
package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
		Symlink: os.Symlink,
		Remove:  os.Remove,
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
	if e.Executable == nil || e.BuildModifier == nil || e.LookPath == nil || e.MkdirAll == nil || e.ReadFile == nil || e.WriteFile == nil || e.Chmod == nil || e.Run == nil || e.RunOutput == nil || e.RunStdin == nil || e.Symlink == nil || e.Remove == nil {
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
		if err := e.WriteFile(voxiPath, data, 0755); err != nil {
			return fmt.Errorf("write %s: %w", voxiPath, err)
		}
		return e.Chmod(voxiPath, 0755)
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
	if err := e.Run(ctx, "curl", "-fL", "-o", archive, url); err != nil {
		return fmt.Errorf("download CrispASR: %w", err)
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

func buildModifier(ctx context.Context, dir string) (string, error) {
	path := filepath.Join(dir, "voxi-modifierd")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	mod, err := exec.CommandContext(ctx, "go", "env", "GOMOD").Output()
	if err != nil || strings.TrimSpace(string(mod)) == "" || strings.TrimSpace(string(mod)) == "/dev/null" {
		return "", fmt.Errorf("no source checkout or packaged voxi-modifierd binary available")
	}
	root := filepath.Dir(strings.TrimSpace(string(mod)))
	cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "./cmd/voxi-modifierd")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return path, nil
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
