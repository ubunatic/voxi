# Robust ASR Fallbacks, Doctor Repairs, and Offline Upstream Resilience

**Date**: 2026-10-22  
**Scope**: `voxi` (desktop voice typing daemon, monitor, doctor) & `lmcoder` (local/distributed LLM & ASR proxy)  
**Starting State**: ASR transcription was completely broken on `t14`; `voxi monitor` showed ASR backend offline; `voxi doctor` passed with 7 OK without noticing broken ASR; `voxi install --modifierd` failed with an opaque `exit status 1`; `lmcoder-proxy.service` was in a rapid crash-restart loop.  
**Intended Goals**: Identify root causes across `voxi` and `lmcoder`; file tracked issues; repair degraded states gracefully when remote upstream (`x600`) is offline; make `voxi doctor` detect and repair ASR faults; surface actionable errors in `voxi install` and `voxi monitor`; enable automated self-healing back to primary backends when upstream returns.

---

## 1. Header & Context

On developer machine `t14` (ThinkPad T14, Linux/Wayland), voice input relies on an eager transcription pipeline connecting to an OpenAI-compatible speech-to-text backend. Under normal multi-machine topology, `~/.config/lmcoder/lmcoder.yaml` configures:

```yaml
upstream: x600
upstream_first: [asr]
```

When `x600` is powered off or unreachable (e.g. mobile work or network split), `t14` must degrade gracefully to its local AMD APU services (`lmcoder-asr.service` on port `8749`, `voxi-r2t2.service` on port `18131`, or CPU `crispasr`). Instead, the user observed a total breakdown:
1. `voxi monitor` reported: `⚠️ ASR backend OFFLINE: http://127.0.0.1:8735/v1 unreachable (dial tcp 127.0.0.1:8735: connect: connection refused)`.
2. `voxi doctor` reported: `7 OK, 0 FAULT`, completely oblivious that dictation was dead.
3. `voxi install --modifierd` failed cryptically: `[modifier daemon (privileged)] failed: sudo install modifier binary: exit status 1`.
4. Running `lmcoder stop`, `lmcoder start`, and `make install` in `../lmcoder` did not recover the proxy.

---

## 2. Executive Summary

This session diagnosed and resolved the end-to-end failure cascade across both repositories:
- **`lmcoder` Issue 196**: The proxy crashed on startup because [`startSSHTunnel`](file:///home/uwe/projects/lmcoder/cmd/lmcoder/tunnel.go) blocked synchronously trying to reach unreachable upstream `x600`. We patched [`runProxy`](file:///home/uwe/projects/lmcoder/cmd/lmcoder/proxy.go) to start in degraded local mode on initial tunnel failure, delegating reconnects to the background supervisor, and updated [`upstreamModelsHandler`](file:///home/uwe/projects/lmcoder/cmd/lmcoder/proxy.go) to serve the local catalog.
- **`voxi` Issue 198**: `DefaultEffects().Run` in [`internal/install/install.go`](file:///home/uwe/projects/voxi/internal/install/install.go) previously discarded `Stderr` and ran with disconnected `Stdin`. Privileged `sudo install` commands failed because the user was prompted for PAM fingerprint authentication without visible output or terminal connection. We attached `os.Stdin` and captured `Stderr` to report exact error strings. We also enriched `ASRBackendState` in [`internal/monitor`](file:///home/uwe/projects/voxi/internal/monitor) with explicit service attribution and remediation commands (`run: voxi doctor --fix`).
- **`voxi` Issue 197**: We extended [`spec/doctor.yaml`](file:///home/uwe/projects/voxi/spec/doctor.yaml) and schema with an `asr-engine` check, implemented automated repair plans in [`internal/doctor/asr.go`](file:///home/uwe/projects/voxi/internal/doctor/asr.go), added a scriptable `voxi settings set` command, and implemented multi-tier runtime fallback in [`internal/eager/openai_transcribe.go`](file:///home/uwe/projects/voxi/internal/eager/openai_transcribe.go) alongside background self-healing auto-reversion in [`internal/agent`](file:///home/uwe/projects/voxi/internal/agent).

Live validation confirmed 100% health: `voxi doctor` reports `8 OK, 0 FAULT`, `voxi settings test` reports `5 passed, 0 failed`, and `lmcoder-proxy.service` is actively serving local ASR over port 8735.

---

## 3. What Worked Well

1. **Parallel Subagent Triage**:
   Using `HTO=0 harnez agent start --model agy:flash38:low`, we dispatched three subagents simultaneously:
   - `worker-lmcoder-196`: Explored proxy startup tunnel failure and catalog degradation.
   - `worker-voxi-197`: Explored doctor spec checks, fallback repair paths, and auto-reversion.
   - `worker-voxi-198`: Explored `install` stderr capture and monitor diagnostic messages.
   All three subagents completed within ~2 minutes, identifying exact lines of code and concrete patches, allowing the orchestrator to synthesize the solution with zero wasted exploration turns.
2. **Atomic Binary Installation under `ETXTBUSY`**:
   When replacing the running `lmcoder` daemon binary, `cp` failed with `Text file busy`. Using `install -m 0755` unlinked the existing inode and created a fresh one atomically, avoiding process lock contention.
3. **Multi-Tiered Resiliency Hierarchy**:
   Instead of relying on a single failure point, the system now features four layers of defense:
   - *Layer 1 (Lmcoder Proxy)*: Resilient proxy startup with background SSH reconnect loop.
   - *Layer 2 (Eager Streaming)*: Transparent client fallback to secondary local ports (18131, 8749) if 8735 fails during a session.
   - *Layer 3 (Doctor Diagnostics)*: `voxi doctor` identifies broken endpoints and provides automated repair options.
   - *Layer 4 (Watchdog Self-Healing)*: `voxi-agent` automatically restores primary models and stops fallback services when normal conditions return.

---

## 4. Honest Post-Mortem (Failures, Bugs & Near-Misses)

### 1. Catalog Merging Regression in `lmcoder`
- **What happened**: Initially, we modified `upstreamModelsHandler` in `cmd/lmcoder/proxy.go` to always return local models whenever the upstream request failed.
- **How it was caught**: Running `go test ./cmd/lmcoder/...` caught a failure in `TestUpstreamModelsHandlerReturns502WhenTunnelIsDown`. In pure upstream mode (`liveLocal == false`), upstream models failure is strictly specified (Issue 159) to return `502 Bad Gateway`.
- **The fix**: We conditioned the fallback catalog on `liveLocal == true`. When `liveLocal` is false, it preserves the expected 502 error; when `liveLocal` is true (e.g. `upstream_first` or local routes), it degrades cleanly to local models.

### 2. Backwards Base URL Precedence in Doctor
- **What happened**: When writing `checkAsrEngine` in `internal/doctor/doctor.go`, we evaluated:
  `if settings != nil && settings.OpenAIASRBaseURL != "" { baseURL = settings.OpenAIASRBaseURL }`
  This caused `baseURL` to evaluate to `http://127.0.0.1:8090/v1` (the legacy global default in `config.UserSettings`) instead of `r2t2-confucius4`'s model-specific URL (`http://127.0.0.1:8735/v1`).
- **How it was caught**: Running `voxi doctor` live reported: `FAULT asr-engine: r2t2-confucius4 endpoint unreachable @ http://127.0.0.1:8090/v1`.
- **The fix**: We replaced the inline check with the canonical resolution function `eager.ResolveOpenAIASRBaseURL(model.BaseURL, globalURL)`, where model-level `base_url` takes precedence over global fallback URLs.

### 3. Harnez Lint File & Function Length Boundary
- **What happened**: Adding `checkAsrEngine` directly to `internal/doctor/doctor.go` pushed the file to 813 lines (exceeding the 800-line limit) and the function to 102 lines (exceeding the 100-line limit).
- **How it was caught**: `make lint` flagged `internal/doctor/doctor.go` for `go-file-length` and `go-func-length`.
- **The fix**: We extracted the ASR engine check into its own file [`internal/doctor/asr.go`](file:///home/uwe/projects/voxi/internal/doctor/asr.go) and split `checkOpenAIASREngine` into a dedicated helper, reducing `doctor.go` to ~705 lines and satisfying all linter constraints.

---

## 5. Quality & Invariants Audit

| Invariant / Quality Dimension | Evaluation | Evidence |
| :--- | :--- | :--- |
| **Architectural Separation** | PASS | `spec/doctor.yaml` remains the declarative source of truth; runtime diagnostics reside in `internal/doctor/asr.go`; settings manipulation is encapsulated in `internal/settings/command.go`. |
| **Idempotency** | PASS | Running `voxi doctor` or `voxi settings test` repeatedly produces identical output with zero side effects. |
| **Zero Zombie Guarantee** | PASS | Auto-reversion in `voxi-agent` explicitly calls `systemctl --user stop voxi-r2t2.service` upon primary recovery, ensuring no idle `llama-server` instances consume memory in the background. |
| **Subagent Zero Zombie** | PASS | All 3 background subagents (`task-282`, `task-284`, `task-286`) exited cleanly with code 0. |
| **Test Coverage** | PASS | Added unit tests: `TestSSHTunnelSupervisorCloseWithNilProcessDoesNotPanic`, `TestDefaultEffectsRunCapturesStderr`, `TestCheckASRBackendStateOfflineAdvice`, `TestCheckAsrEngine`, and `TestWatchASRFallbackRevert`. Full `make test-short` passes across all 38 packages. |

---

## 6. Efficiency & Velocity Assessment

- **Timeline**:
  - Investigation & root cause discovery: ~10 minutes (accelerated by concurrent subagents).
  - `lmcoder` proxy patch, test, and systemd deployment: ~10 minutes.
  - `voxi` install stderr capture & monitor advice: ~8 minutes.
  - `voxi doctor` ASR check, repairs, and eager runtime fallback: ~12 minutes.
  - End-to-end live verification and retrospective: ~5 minutes.
- **Agent Efficiency**:
  - Offloaded file parsing and spec cross-referencing to `flash38:low` subagents, preserving orchestrator context window and minimizing token expenditure.
  - Zero manual polling loops; relied entirely on reactive task triggers.

---

## 7. Key Learnings & Evergreen Upstream

1. **Subprocess Privilege and PAM Visibility**:
   Any orchestration CLI that invokes `sudo` or privileged system helpers must never blindly discard `Stderr` or disconnect `Stdin`. In Linux environments using `pam_fprintd` or interactive password prompts, suppressing terminal I/O turns interactive authentication into silent, baffling `exit status 1` failures.
2. **Distributed Upstream Graceful Degradation**:
   In multi-host topologies (e.g. edge laptop + server workstation), proxies that route to a remote host over SSH tunnels must never treat initial tunnel connectivity as a fatal startup condition if local fallback models are configured. Startup must succeed in degraded mode, with connection retry relegated to a background supervisor.
3. **Two-Way Self-Healing**:
   Fault tolerance is incomplete if it only fails forward. Workarounds that spawn local fallback engines (such as a local GGUF server on port 18131) must have a companion recovery path that detects when the primary service returns, reverts configuration, and reaps the temporary fallback process.

---

## 8. File & Diff Summary

### Repository: `lmcoder`
- [`cmd/lmcoder/proxy.go`](file:///home/uwe/projects/lmcoder/cmd/lmcoder/proxy.go): Graceful startup degradation when upstream is unreachable; local catalog fallback on `/v1/models`.
- [`cmd/lmcoder/tunnel.go`](file:///home/uwe/projects/lmcoder/cmd/lmcoder/tunnel.go): Rate-limited tunnel reconnection logging and supervisor resilience.
- [`cmd/lmcoder/tunnel_test.go`](file:///home/uwe/projects/lmcoder/cmd/lmcoder/tunnel_test.go): Added test for nil process handling on supervisor close.
- [`issues/196-proxy-crashes-when-upstream-host-is-offline-instead-of-degrading-gracefully.md`](file:///home/uwe/projects/lmcoder/issues/196-proxy-crashes-when-upstream-host-is-offline-instead-of-degrading-gracefully.md): Documented and closed issue 196.

### Repository: `voxi`
- [`spec/doctor.yaml`](file:///home/uwe/projects/voxi/spec/doctor.yaml) & [`spec/schemas/doctor.schema.json`](file:///home/uwe/projects/voxi/spec/schemas/doctor.schema.json): Added `asr-engine` check declaration and repair templates.
- [`internal/doctor/asr.go`](file:///home/uwe/projects/voxi/internal/doctor/asr.go): Implemented `checkAsrEngine` and `checkOpenAIASREngine` with repair suggestions.
- [`internal/doctor/doctor.go`](file:///home/uwe/projects/voxi/internal/doctor/doctor.go) & [`internal/doctor/fix.go`](file:///home/uwe/projects/voxi/internal/doctor/fix.go): Wired `asr-engine` dispatch and enabled repair execution for warnings and faults.
- [`internal/doctor/doctor_test.go`](file:///home/uwe/projects/voxi/internal/doctor/doctor_test.go): Added `TestCheckAsrEngine` test suite.
- [`internal/install/install.go`](file:///home/uwe/projects/voxi/internal/install/install.go) & [`internal/install/install_test.go`](file:///home/uwe/projects/voxi/internal/install/install_test.go): Captured stderr in `Run` and `RunStdin`; attached `os.Stdin` for interactive sudo prompts; added unit test.
- [`internal/monitor/collector.go`](file:///home/uwe/projects/voxi/internal/monitor/collector.go), [`internal/monitor/render.go`](file:///home/uwe/projects/voxi/internal/monitor/render.go), [`internal/monitor/asr_probe_test.go`](file:///home/uwe/projects/voxi/internal/monitor/asr_probe_test.go): Enriched ASR offline diagnostics with service attribution and repair advice.
- [`internal/settings/command.go`](file:///home/uwe/projects/voxi/internal/settings/command.go): Added non-interactive `voxi settings set KEY VALUE` command.
- [`internal/eager/openai_transcribe.go`](file:///home/uwe/projects/voxi/internal/eager/openai_transcribe.go): Added client failover to local fallback endpoints when port 8735 is unreachable.
- [`internal/agent/command.go`](file:///home/uwe/projects/voxi/internal/agent/command.go) & [`internal/agent/agent_test.go`](file:///home/uwe/projects/voxi/internal/agent/agent_test.go): Added `watchASRFallbackRevert` background loop to restore default model and stop `voxi-r2t2.service` once primary endpoint recovers.
- [`issues/197-robust-asr-fallbacks-and-self-healing-for-offline-upstream-and-doctor-repair.md`](file:///home/uwe/projects/voxi/issues/197-robust-asr-fallbacks-and-self-healing-for-offline-upstream-and-doctor-repair.md): Documented and closed issue 197.
- [`issues/198-improve-error-reporting-in-voxi-install-and-monitor-for-privileged-backend-failures.md`](file:///home/uwe/projects/voxi/issues/198-improve-error-reporting-in-voxi-install-and-monitor-for-privileged-backend-failures.md): Documented and closed issue 198.
