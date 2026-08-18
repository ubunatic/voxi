package deps

import (
	"context"
	"io"
	"os"
	"os/exec"
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
	Sleep     func(time.Duration)
	Stdin     io.Reader
	Stdout    io.Writer
}

// DefaultDependencies provides production host bindings.
func DefaultDependencies(in io.Reader, out io.Writer) Dependencies {
	return Dependencies{
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		Getenv:   os.Getenv,
		ReadFile: os.ReadFile,
		Stat:     os.Stat,
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
		RunOutput: func(ctx context.Context, name string, args ...string) (string, error) {
			out, err := exec.CommandContext(ctx, name, args...).Output()
			return string(out), err
		},
		RunStdin: func(ctx context.Context, stdin string, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdin = strings.NewReader(stdin)
			return cmd.Run()
		},
		Sleep:  time.Sleep,
		Stdin:  in,
		Stdout: out,
	}
}
