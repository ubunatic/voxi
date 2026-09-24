#!/usr/bin/env python3
"""Check whether a direct playback child dies when its monitor owner is SIGKILLed."""

import ctypes
import os
import signal
import subprocess
import sys
import time

PR_SET_PDEATHSIG = 1


def child(parent_pid):
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.prctl(PR_SET_PDEATHSIG, signal.SIGTERM, 0, 0, 0) != 0:
        raise OSError(ctypes.get_errno(), "prctl(PR_SET_PDEATHSIG)")
    if os.getppid() != parent_pid:
        raise RuntimeError("owner exited before child installed its parent-death signal")
    print(f"CHILD_PID={os.getpid()}", flush=True)
    while True:
        time.sleep(10)


def owner():
    child_proc = subprocess.Popen(
        [sys.executable, __file__, "child", str(os.getpid())],
        stdout=subprocess.PIPE,
        text=True,
        start_new_session=True,
    )
    print(child_proc.stdout.readline().strip(), flush=True)
    child_proc.wait()


def main():
    if len(sys.argv) > 1 and sys.argv[1] == "child":
        child(int(sys.argv[2]))
        return
    owner_proc = subprocess.Popen(
        [sys.executable, __file__, "owner"], stdout=subprocess.PIPE, text=True
    )
    line = owner_proc.stdout.readline().strip()
    child_pid = int(line.removeprefix("CHILD_PID="))
    os.kill(owner_proc.pid, signal.SIGKILL)
    owner_proc.wait()
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        try:
            with open(f"/proc/{child_pid}/stat", encoding="ascii") as stat_file:
                state = stat_file.read().split()[2]
            if state == "Z":
                break
        except (FileNotFoundError, ProcessLookupError):
            break
        time.sleep(0.05)
    else:
        raise SystemExit(f"FAIL: playback child {child_pid} survived owner SIGKILL")
    print(f"PASS: direct playback child {child_pid} exited after owner SIGKILL")


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "owner":
        owner()
    else:
        main()
