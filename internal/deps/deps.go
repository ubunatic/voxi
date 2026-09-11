package deps

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Dependencies isolates host reads and subprocesses for tests.
type Dependencies struct {
	GOOS      string
	GOARCH    string
	Getenv    func(string) string
	ReadFile  func(string) ([]byte, error)
	Stat      func(string) (os.FileInfo, error)
	LookPath  func(string) (string, error)
	Run       func(context.Context, string, ...string) error
	RunOutput func(ctx context.Context, name string, args ...string) (string, error)
	RunStdin  func(ctx context.Context, stdin string, name string, args ...string) error
	// RunStdinProcess is the lifecycle-aware stdin boundary used by desktop
	// injection. It returns the child PID after the process has exited. A nil
	// value falls back to RunStdin for lightweight callers and tests.
	RunStdinProcess func(ctx context.Context, stdin string, name string, args ...string) (pid int, err error)
	Sleep           func(time.Duration)
	// AfterAudioRead is an optional test seam invoked after a capture frame
	// has been read and timestamped, before cancellation/processing checks.
	AfterAudioRead func()
	Stdin          io.Reader
	Stdout         io.Writer
}

// lookPathWithFallbacks resolves binaries via standard exec.LookPath, falling back to
// standard user binary locations (~/.local/bin, ~/go/bin) if not present on PATH.
func lookPathWithFallbacks(file string) (string, error) {
	if lp, err := exec.LookPath(file); err == nil {
		return lp, nil
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates := []string{
			filepath.Join(home, ".local", "bin", file),
			filepath.Join(home, "go", "bin", file),
		}
		for _, cand := range candidates {
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && (fi.Mode()&0111 != 0) {
				return cand, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

// resolveCommand returns the full path or original name.
func resolveCommand(name string) string {
	if p, err := lookPathWithFallbacks(name); err == nil {
		return p
	}
	return name
}

// DefaultDependencies provides production host bindings.
func DefaultDependencies(in io.Reader, out io.Writer) Dependencies {
	return Dependencies{
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		Getenv:   os.Getenv,
		ReadFile: os.ReadFile,
		Stat:     os.Stat,
		LookPath: lookPathWithFallbacks,
		Run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, resolveCommand(name), args...).Run()
		},
		RunOutput: func(ctx context.Context, name string, args ...string) (string, error) {
			out, err := exec.CommandContext(ctx, resolveCommand(name), args...).Output()
			return string(out), err
		},
		RunStdin: func(ctx context.Context, stdin string, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, resolveCommand(name), args...)
			cmd.Stdin = strings.NewReader(stdin)
			return cmd.Run()
		},
		RunStdinProcess: func(ctx context.Context, stdin string, name string, args ...string) (int, error) {
			cmd := exec.CommandContext(ctx, resolveCommand(name), args...)
			cmd.Stdin = strings.NewReader(stdin)
			if err := cmd.Start(); err != nil {
				return 0, err
			}
			pid := cmd.Process.Pid
			return pid, cmd.Wait()
		},
		Sleep:  time.Sleep,
		Stdin:  in,
		Stdout: out,
	}
}
