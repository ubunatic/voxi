# 035: SSH-Managed Remote Voxi Transcription Server Research

**Status**: Proposed / Research  
**Priority**: P2 (Medium)  
**Severity**: Moderate  
**Category**: Architecture  
**Related**: [eager VAD/transcription flow](../internal/eager/eager.go), [systemd eager unit](../systemd/voxi-eager.service), [model benchmark baseline](../docs/BenchBaseline.md)

---

## 1. Goal

Assess an opt-in Voxi architecture in which the desktop keeps microphone
capture, 16 kHz PCM VAD segmentation, hotkeys, local feedback filtering,
history, and Wayland typing, while a more capable remote machine—initially
**`x600` only**—runs the Whisper/Voxtype inference service.

The only deployment and transport authority in scope is SSH. The eventual
feature should make it practical to deploy/update the remote transcription
unit from the Voxi CLI, but this ticket is research only.

## 2. Intended Boundary

```text
desktop: microphone -> VAD/pre-roll -> completed WAV utterance
                                      |
                                      | authenticated SSH tunnel
                                      v
x600: loopback-only Voxi ASR service -> transcript + timing JSON
                                      |
desktop: ASR filters/feedback -> history -> dotool typing
```

The remote unit must never access a microphone, PipeWire/ALSA device,
`/dev/uinput`, dotool, desktop focus, or desktop history. It receives only a
completed utterance and returns a transcript; it does not stream keystrokes or
own a remote VAD loop.

## 3. Research Questions

### 3.1 Protocol and latency

- What is the smallest RPC contract: `POST /transcribe` with WAV body plus
  model/language/prompt options, returning text, elapsed time, and error?
- Can the local eager worker retain its sequential backpressure and cancellation
  behaviour across a persistent SSH tunnel?
- Compare two SSH-only transport candidates:
  1. remote `voxi-transcribed` binds only `127.0.0.1`; desktop opens a managed
     `ssh -N -L localport:127.0.0.1:remoteport x600` tunnel;
  2. a persistent `ssh x600 voxi transcribe --stdio` session uses framed
     request/response messages and exposes no remote TCP listener.
- Canary each path with representative WAVs; record connection setup time,
  utterance round-trip latency, RTF, cancellation behaviour, reconnection time,
  and concurrent-request policy.

### 3.2 Remote deployment

Assess a deliberately narrow command, conceptually:

```text
voxi remote deploy x600
voxi remote status x600
voxi remote logs x600
```

`deploy` would use SSH to verify host identity, architecture, available model
backend/GPU, disk/RAM, and user systemd availability; copy the versioned Voxi
binary and a generated **transcription-only** user service; run
`systemctl --user daemon-reload`, `enable --now`, and report a health check.
It must be idempotent and refuse an unrecognized host until the user configures
it. No generic arbitrary-host deployment is in the first slice.

Determine whether x600 requires `loginctl enable-linger` for a durable user
service; never enable it implicitly. Record the exact manual prerequisite.

### 3.3 Security and privacy

- Reuse normal SSH host-key verification and the user's existing `x600` SSH
  configuration. Do not add passwords, an HTTP listener on the LAN, a public
  port, or a second bespoke authentication scheme.
- Bind any HTTP service to remote loopback only; the tunnel is the sole ingress.
- Treat audio and resulting text as sensitive. Keep temporary WAVs in a
  private runtime directory, delete on completion/cancel, avoid request-body
  logging, and redact transcript content from remote journal logs by default.
- Define a clear local fallback when the tunnel/host/model is unavailable;
  failure must neither block microphone shutdown nor type stale text.

## 4. Feasibility Assessment (Initial)

The **deployment slice is low-to-medium complexity**: Voxi already builds a
single binary and installs user systemd units. The new work is SSH capability
probing, atomic remote copy/version rollback, a service template without audio
or typing privileges, and clear diagnostics.

The **remote-ASR client/server slice is medium complexity**: eager mode's
`TranscribeJob` boundary already contains a completed audio buffer and timing,
so it can be abstracted behind a transcriber interface. The difficult parts are
not audio capture but cancellation, transport framing, model warm-up, safe
failure fallback, and benchmark/canary coverage. A loopback HTTP server through
an SSH tunnel is likely the easiest observable first canary; stdio framing may
be simpler to secure but needs more custom lifecycle code.

No implementation should start until the canary selects one transport and
measures whether x600 improves end-to-end latency/accuracy enough to justify
moving audio off the desktop.

## 5. Deliverables

1. Record x600's OS, architecture, GPU/runtime, `systemd --user`, SSH, model
   storage, and inference benchmark evidence.
2. Implement only a throwaway canary for the selected SSH transport before
   creating production server/client code.
3. Document the proposed RPC schema, failure states, and exact service
   hardening/installation commands.
4. Create a separate implementation ticket only if the canary passes the
   latency, privacy, and operational acceptance gates.

## 6. Non-goals

- No remote microphone capture, VAD, typing, modifier daemon, GNOME extension,
  or history persistence.
- No cloud hosting, VPN, reverse proxy, general fleet manager, or non-SSH
  deployment method.
- No automatic SSH trust, host-key bypass, linger enablement, or deployment to
  a host other than explicitly configured `x600`.
