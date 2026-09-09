# 052: CPU and GPU Scheduling Priority Research for Responsive Dictation Under Load

**Status**: Closed — Folded into 088 — same launch path (systemd voxi-agent.service), same question (scheduling priority under load). Research questions carried into 088's investigation section
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Research
**Related**: [050 optional warm-model daemon transcription](050-optional-warm-model-daemon-transcription.md), [051 warm-model prior art research](051-warm-model-prior-art-research.md), [systemd/voxi-agent.service](../systemd/voxi-agent.service)

---

## 1. Problem & Motivation

When the workstation is under high CPU or GPU load (e.g. compiling large codebases, running local LLMs, training/fine-tuning tasks, browser rendering, or background jobs), Voxi typing responsiveness degrades significantly.

Voice typing is an interactive, real-time desktop interface: human expectations for typing latency are on the order of 100–300 ms. A delay of several seconds due to CPU starvation or GPU compute queue contention makes voice dictation feel sluggish or frozen.

Currently:
1. `voxi-agent.service` runs as a standard `systemd --user` service with default scheduling priority (nice value 0, standard `SCHED_OTHER` scheduling class, no cgroup CPU weight guarantees).
2. Transcription subprocesses (`voxtype transcribe ...`) inherit this default priority.
3. On the GPU side (AMD Vulkan/RADV or NVIDIA CUDA), `voxtype` / `whisper.cpp` dispatches compute kernels without explicit queue priority flags or context priority, so its work queues behind or interleaves fairly with bulk compute workloads.

This ticket investigates how Voxi and its transcription workers can be prioritized on both CPU and GPU so that interactive dictation remains fluid even when the machine is heavily loaded.

---

## 2. Research Questions

### 2.1 CPU Priority & Scheduling

1. **Systemd User Service Directives**:
   - What scheduling directives can be added to `systemd/voxi-agent.service`?
     - `Nice=-10` or `Nice=-5` (requires PAM limits or `LimitNICE` permissions in systemd user session).
     - `CPUSchedulingPolicy=rr` / `fifo` vs `batch` / `other`. Real-time scheduling policies (`SCHED_RR`, `SCHED_FIFO`) or low-latency interactive (`SCHED_OTHER` with negative nice).
     - `LimitNICE=` configuration for user services.
   - What are the permission limits for unprivileged user services under standard systemd (e.g., Ubuntu/Fedora default `/etc/security/limits.conf` or polkit)?
2. **Process Inheritance**:
   - When `voxi agent` spawns `voxtype` via `exec.CommandContext(...)`, do the nice values and scheduling classes cleanly inherit to the child process and its OpenMP/pthreads thread pools?
   - Can Go code adjust the nice value or call `syscall.Setpriority` explicitly on child processes or before execution (via `SysProcAttr`)?
3. **cgroups v2 CPU Weighting**:
   - Does systemd user slice support `CPUWeight=500` or `StartupCPUWeight=` to guarantee proportional CPU share under CPU starvation?

### 2.2 GPU Priority & Compute Preemption

1. **Vulkan / RADV (AMD)**:
   - `voxtype` on this workstation builds with Vulkan compute (`RADV`).
   - Vulkan provides extensions for queue and context priorities:
     - `VK_EXT_global_priority` / `VK_EXT_global_priority_query` (allows requesting `VK_QUEUE_GLOBAL_PRIORITY_HIGH_EXT` or `REALTIME`).
     - Note: `REALTIME` typically requires `CAP_SYS_NICE` or specific DRM permissions, but `HIGH` may be accessible or configurable via environment variables / driver configs.
   - Does Mesa/RADV have environment variables (e.g., `RADV_DEBUG`, `MESA_VK_DEVICE_SELECT`, or priority overrides) or DRI render node scheduling controls?
2. **DRM / Linux Kernel GPU Scheduling**:
   - AMDGPU kernel driver uses GPU scheduler (`amdgpu_sched`) with priority rings (low, normal, high, real-time).
   - Can client processes request high-priority DRM contexts without root privileges?
3. **whisper.cpp / voxtype flags**:
   - Does upstream `whisper.cpp` or `voxtype` expose any queue priority or compute stream configuration?
   - If using the HTTP daemon approach from issue 050/051, does keeping the context resident in VRAM reduce GPU bus contention and pipeline stalls caused by reloading buffers under memory bandwidth pressure?

### 2.3 Audio Capture Priority (PipeWire / ALSA)

1. When CPU load is high, buffer underruns (xruns) can cause audio dropouts before transcription even starts.
2. PipeWire's client latency (`PIPEWIRE_LATENCY`) and RTKit / `rtkit-daemon` integration:
   - Does Voxi's audio recording thread obtain real-time priority via `rtkit`?
   - How can we verify that the capture path is protected against xruns under load?

---

## 3. Investigation Plan & Canary Probes

1. **Probe CPU Nice and User Limits**:
   - Test `systemd --user` with `Nice=-5` or `Nice=-10` in `voxi-agent.service`. Check `systemctl --user daemon-reload` and journal for permission errors.
   - Verify child process nice levels with `ps -eo pid,ni,comm | grep -E 'voxi|voxtype'`.
2. **Simulate Synthetic Load Benchmark**:
   - Run a stress test (e.g., `stress-ng --cpu 8 --io 4` or a GPU compute loop) while running `voxi feedback sample play` / `transcribe` on `kt-sentences-plus-silence`.
   - Compare Real-Time Factor (RTF) and transcription latency with default priority vs elevated CPU/IO priority.
3. **Investigate Vulkan Compute Priority on RADV**:
   - Inspect RADV / kernel support for `VK_EXT_global_priority` on the host GPU (`vulkaninfo | grep VK_EXT_global_priority`).
   - Check if unprivileged processes can open high-priority contexts.
4. **Evaluate System Resource Limits**:
   - Document required configuration (e.g. `/etc/security/limits.d/99-voxi.conf` if `LimitNICE` requires elevated user ceiling).

---

## 4. Deliverables

- Detailed technical findings on what user-level CPU and GPU priority mechanisms are supported on modern Linux/Wayland without requiring root privileges or destabilizing desktop graphics.
- Concrete recommendations for `systemd/voxi-agent.service` (e.g. `Nice`, `CPUWeight`, `IOWeight`).
- Recommendations for `internal/eager` subprocess invocation attributes (`SysProcAttr`).
