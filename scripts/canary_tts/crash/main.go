// A live canary for supervisor cleanup of text2wave's Festival grandchild.
// Run with: go run ./scripts/canary_tts/crash /path/to/voxi
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type process struct {
	pid   int
	ppid  int
	pgid  int
	state byte
	name  string
	args  string
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "owner" {
		owner(os.Args[2:])
		return
	}
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: crash <voxi-binary>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

func run(voxi string) error {
	textPath := filepath.Join(os.TempDir(), fmt.Sprintf("voxi-tts-crash-%d.txt", os.Getpid()))
	wavPath := filepath.Join(os.TempDir(), fmt.Sprintf("voxi-tts-crash-%d.wav", os.Getpid()))
	defer os.Remove(textPath)
	defer os.Remove(wavPath)
	text := strings.Repeat("Festival descendant cleanup canary. This sentence keeps synthesis active. ", 300)
	if err := os.WriteFile(textPath, []byte(text), 0600); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	ownerCmd := exec.Command(self, "owner", voxi, textPath, wavPath)
	ownerOut, err := ownerCmd.StdoutPipe()
	if err != nil {
		return err
	}
	ownerCmd.Stderr = os.Stderr
	if err := ownerCmd.Start(); err != nil {
		return err
	}
	ownerPID := ownerCmd.Process.Pid
	ownerKilled := false
	defer func() {
		if !ownerKilled {
			_ = ownerCmd.Process.Kill()
			_ = ownerCmd.Wait()
		}
	}()
	line, err := bufio.NewReader(ownerOut).ReadString('\n')
	if err != nil {
		return fmt.Errorf("owner did not start supervisor: %w", err)
	}
	helpPID, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "SUPERVISOR_PID=")))
	if err != nil || helpPID <= 0 {
		return fmt.Errorf("invalid supervisor readiness %q", line)
	}
	_ = helpPID
	pgid, festivalPID, err := waitForFestival(ownerPID, 15*time.Second)
	if err != nil {
		return err
	}
	if err := ownerCmd.Process.Kill(); err != nil {
		return fmt.Errorf("SIGKILL owner: %w", err)
	}
	ownerKilled = true
	_ = ownerCmd.Wait()
	if err := waitForGroupExit(pgid, 5*time.Second); err != nil {
		return err
	}
	fmt.Printf("PASS: owner SIGKILL stopped Festival pid %d and left process group %d empty\n", festivalPID, pgid)
	return nil
}

func owner(args []string) {
	if len(args) != 3 {
		os.Exit(2)
	}
	command, err := exec.LookPath("text2wave")
	if err != nil {
		fmt.Fprintln(os.Stderr, "text2wave is not installed")
		os.Exit(1)
	}
	supervisor := exec.Command(args[0], "__tts-supervise", "--", command, args[1], "-o", args[2])
	supervisor.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	if err := supervisor.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("SUPERVISOR_PID=%d\n", supervisor.Process.Pid)
	if err := supervisor.Wait(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func waitForFestival(root int, timeout time.Duration) (int, int, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		procs := processTable()
		for _, p := range procs {
			if !strings.Contains(strings.ToLower(p.name+" "+p.args), "festival") || p.state == 'Z' {
				continue
			}
			for parent := p.ppid; parent != 0; {
				if parent == root {
					return p.pgid, p.pid, nil
				}
				ancestor, ok := procs[parent]
				if !ok || ancestor.ppid == parent {
					break
				}
				parent = ancestor.ppid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0, 0, fmt.Errorf("real Festival descendant did not appear within %s", timeout)
}

func waitForGroupExit(pgid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		alive := false
		for _, p := range processTable() {
			if p.pgid == pgid && p.state != 'Z' {
				alive = true
				break
			}
		}
		if !alive {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("process group %d still has live members after owner SIGKILL", pgid)
}

func processTable() map[int]process {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	procs := make(map[int]process)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		p, err := readProcess(pid)
		if err == nil {
			procs[pid] = p
		}
	}
	return procs
}

func readProcess(pid int) (process, error) {
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return process{}, err
	}
	closeParen := strings.LastIndexByte(string(statData), ')')
	if closeParen < 0 {
		return process{}, fmt.Errorf("invalid proc stat")
	}
	nameStart := strings.IndexByte(string(statData), '(')
	fields := strings.Fields(string(statData[closeParen+1:]))
	if nameStart < 0 || len(fields) < 3 {
		return process{}, fmt.Errorf("invalid proc stat fields")
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return process{}, err
	}
	pgid, err := strconv.Atoi(fields[2])
	if err != nil {
		return process{}, err
	}
	argsData, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	return process{pid: pid, ppid: ppid, pgid: pgid, state: fields[0][0], name: string(statData[nameStart+1 : closeParen]), args: strings.ReplaceAll(string(argsData), "\x00", " ")}, nil
}
