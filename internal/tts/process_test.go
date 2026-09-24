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
)

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
