package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// ── Process Manager ──────────────────────────────────────────────────────────

type processManager struct {
	cmds []*exec.Cmd
}

func (pm *processManager) add(cmd *exec.Cmd) {
	pm.cmds = append(pm.cmds, cmd)
}

func (pm *processManager) killAll() {
	for i := len(pm.cmds) - 1; i >= 0; i-- {
		cmd := pm.cmds[i]
		if cmd != nil && cmd.Process != nil {
			pgid, err := syscall.Getpgid(cmd.Process.Pid)
			if err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGTERM)
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			}
			_ = cmd.Process.Kill()
		}
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func detectEditor() (string, error) {
	for _, candidate := range []string{"gedit", "gnome-text-editor"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("neither gedit nor gnome-text-editor found in PATH")
}

func killStaleNestedProcesses() {
	// Find and kill stale gnome-shell instances that were run with --nested or --devkit
	out, err := exec.Command("pgrep", "-a", "gnome-shell").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "--nested") || strings.Contains(line, "--devkit") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					var pid int
					if _, err := fmt.Sscanf(fields[0], "%d", &pid); err == nil && pid > 0 && pid != os.Getpid() {
						_ = syscall.Kill(pid, syscall.SIGKILL)
					}
				}
			}
		}
	}
	_ = exec.Command("pkill", "-f", "gnome-shell-calendar-server").Run()
}

func gnomeShellModeFlag() string {
	out, err := exec.Command("gnome-shell", "--help").CombinedOutput()
	if err == nil && bytes.Contains(out, []byte("--devkit")) {
		return "--devkit"
	}
	return "--nested"
}

func sendDotoolInput(input string) error {
	var cmd *exec.Cmd
	if _, err := os.Stat("/tmp/dotool-pipe"); err == nil {
		if _, err := exec.LookPath("dotoolc"); err == nil {
			cmd = exec.Command("dotoolc")
		}
	}
	if cmd == nil {
		if _, err := exec.LookPath("dotool"); err == nil {
			cmd = exec.Command("dotool")
		} else {
			return errors.New("neither dotoolc nor dotool found in PATH")
		}
	}

	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func detectWaylandSocket(logPath string, timeout time.Duration) (string, error) {
	socketRe := regexp.MustCompile(`wayland-[0-9]+`)
	deadline := time.Now().Add(timeout)
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	for time.Now().Before(deadline) {
		// 1. Scan gnome-shell log file
		if data, err := os.ReadFile(logPath); err == nil {
			if match := socketRe.Find(data); len(match) > 0 {
				return string(match), nil
			}
		}

		// 2. Scan XDG runtime dir for wayland-1, wayland-2, etc.
		entries, err := filepath.Glob(filepath.Join(runtimeDir, "wayland-[1-9]*"))
		if err == nil {
			for _, entry := range entries {
				if fi, err := os.Stat(entry); err == nil && (fi.Mode()&os.ModeSocket != 0) {
					return filepath.Base(entry), nil
				}
			}
		}

		time.Sleep(250 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout waiting for nested Wayland socket (see log: %s)", logPath)
}

// ── Main Canary Execution ────────────────────────────────────────────────────

func runCanary(ctx context.Context) error {
	editorPath, err := detectEditor()
	if err != nil {
		return err
	}

	for _, dep := range []string{"gnome-shell", "dbus-run-session"} {
		if _, err := exec.LookPath(dep); err != nil {
			return fmt.Errorf("missing required dependency: %s", dep)
		}
	}

	tmpDir, err := os.MkdirTemp("", "canary-nested-shell-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	logPath := filepath.Join(tmpDir, "nested-shell.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()

	pm := &processManager{}
	defer func() {
		fmt.Printf("\nCleaning up child processes...\n")
		pm.killAll()
	}()

	// Kill stale nested instances before starting fresh
	killStaleNestedProcesses()

	// 1. Start nested GNOME Shell
	fmt.Println("1. Launching nested GNOME Shell...")
	shellFlag := gnomeShellModeFlag()
	shellCmd := exec.Command("dbus-run-session", "--", "gnome-shell", shellFlag, "--wayland")
	shellCmd.Env = append(os.Environ(), "MUTTER_DEBUG_DUMMY_MODE_SPECS=1280x720@60.0")
	shellCmd.Stdout = logFile
	shellCmd.Stderr = logFile
	shellCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := shellCmd.Start(); err != nil {
		return fmt.Errorf("start nested gnome-shell: %w", err)
	}
	pm.add(shellCmd)

	fmt.Println("   Waiting for nested Wayland socket initialization...")
	waylandDisplay, err := detectWaylandSocket(logPath, 12*time.Second)
	if err != nil {
		return err
	}
	fmt.Printf("  ✓ Nested GNOME Shell active on %s (PID: %d)\n", waylandDisplay, shellCmd.Process.Pid)

	// 2. Start editor inside nested display
	savedFile := filepath.Join(tmpDir, "saved-dictation.txt")
	fmt.Printf("2. Spawning %s on %s with target %s...\n", filepath.Base(editorPath), waylandDisplay, savedFile)

	editorCmd := exec.Command(editorPath, savedFile)
	editorCmd.Env = append(os.Environ(), "WAYLAND_DISPLAY="+waylandDisplay)
	editorCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := editorCmd.Start(); err != nil {
		return fmt.Errorf("start editor: %w", err)
	}
	pm.add(editorCmd)
	fmt.Printf("  ✓ Editor started (PID: %d)\n", editorCmd.Process.Pid)

	// 3. User focus prompt & countdown
	token := fmt.Sprintf("CanaryVerificationToken_%d", time.Now().Unix())
	fmt.Printf("\n>>> ACTION REQUIRED: Click inside the nested %s window to focus it! <<<\n", filepath.Base(editorPath))
	for i := 3; i > 0; i-- {
		fmt.Printf("Injecting in %d seconds...\r", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println()

	// 4. Inject synthetic typing and Ctrl+S save
	fmt.Printf("3. Injecting text: %q\n", token)
	typePayload := fmt.Sprintf("type %s\nkey enter\n", token)
	if err := sendDotoolInput(typePayload); err != nil {
		return fmt.Errorf("send text: %w", err)
	}

	time.Sleep(800 * time.Millisecond)
	fmt.Println("   Sending Ctrl+S shortcut...")
	if err := sendDotoolInput("key ctrl+s\n"); err != nil {
		return fmt.Errorf("send ctrl+s: %w", err)
	}

	time.Sleep(1200 * time.Millisecond)

	// 5. Verify saved file content
	fmt.Println("4. Verifying saved file content on disk...")
	content, err := os.ReadFile(savedFile)
	if err == nil && strings.Contains(string(content), token) {
		fmt.Printf("  ✓ Verification SUCCESS: Token found in %s\n", savedFile)
		fmt.Println("   Sending Ctrl+Q to close editor...")
		_ = sendDotoolInput("key ctrl+q\n")
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	fmt.Printf("  [WARN] Token not yet found in %s.\n", savedFile)
	fmt.Printf("         (Check if the nested editor window was focused during countdown)\n")

	fmt.Print("\nCanary finished. Press Enter to clean up and exit: ")
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')

	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := runCanary(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
