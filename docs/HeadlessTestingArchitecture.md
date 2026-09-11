# Headless Testing & Container Isolation Architecture

**Status:** Accepted  
**Date:** 2026-09-11  
**Related:** [Issue 105](../issues/105-headless-end-to-end-integration-test-with-virtual-audio-and-uinput-in-container.md), [Installation Architecture](InstallationArchitecture.md), [Eager Delivery Safety](EagerDeliverySafety.md), [scripts/test-e2e-headless.sh](../scripts/test-e2e-headless.sh)

---

## 1. Motivation & Challenge

Testing the live voice-input runtime pipeline end-to-end requires exercising:
1. Real audio capture from an input stream (`arecord` / `pw-record`),
2. Real Voice Activity Detection (VAD) audio segmentation,
3. Real neural speech-to-text inference (`crispasr` / Cohere Transcribe), and
4. Synthetic desktop keystroke injection (`dotool` / `dotoold`).

In developer environments and CI runners, physical microphones, dedicated audio cards, and active desktop displays are typically absent. Furthermore, running end-to-end typing tests on developer workstations must never inject synthetic keystrokes into active IDE or chat windows.

---

## 2. Virtual Boundaries & Architecture

The headless testing harness (`scripts/test-e2e-headless.sh`, `make test-e2e`) runs in an isolated container (Podman) with virtualized boundaries:

```
┌────────────────────────────────────────────────────────────────────────┐
│ Container Test Harness (Podman)                                        │
│                                                                        │
│   [ paplay fixture.wav ]                                               │
│             │                                                          │
│             ▼                                                          │
│   [ PulseAudio auto_null.monitor ] ──(virtual mic)──► [ arecord ]      │
│                                                            │           │
│                                                            ▼           │
│                                                   [ voxi eager ]       │
│                                                      ├─ VAD Chunking   │
│                                                      ├─ CrispASR STT   │
│                                                      └─ dotoolc Client │
│                                                            │           │
│                                                            ▼           │
│   [ FIFO Sink: /tmp/dotool-pipe (O_RDWR) ] ◄───────────────┘           │
│             │                                                          │
│             ▼                                                          │
│   [ Injected Keystroke Verification ]                                  │
└────────────────────────────────────────────────────────────────────────┘
```

### 2.1 Virtual Audio Source (Microphone Simulation)

Rather than relying on host ALSA kernel modules (`snd-dummy`), the test harness initializes an unprivileged userspace PulseAudio daemon with a null sink:
- Default capture source: `auto_null.monitor`.
- Audio playback (`paplay /workspace/test/fixtures/short-one-two.wav`) streams reference PCM audio into the sink in real time.
- `voxi eager` spawns `arecord`, capturing 16kHz mono S16_LE PCM from the virtual source identically to a physical microphone.

### 2.2 Typing Isolation & The uinput Host Leak Pitfall

> [!WARNING]
> In the Linux kernel, `/dev/uinput` is **not namespaced** across container boundaries.

When `--device /dev/uinput` is mounted into a container, `dotoold` creates a virtual evdev keyboard on the **host kernel's input table**. The host desktop compositor (GNOME Shell / Mutter) immediately attaches to this device and delivers injected keystrokes to whichever host window currently has keyboard focus.

**Solution:**
- The container test harness deliberately does **not** map host `/dev/uinput`.
- The typing boundary is isolated using a persistent FIFO on `/tmp/dotool-pipe`.
- `voxi eager` and `dotoolc` format and transmit the exact `type <text>` command stream into the FIFO.
- A background listener captures and asserts the generated keystroke stream, guaranteeing 100% test fidelity with 0% risk of host desktop interference.

### 2.3 Persistent FIFO Descriptor Lifecycle

In standard UNIX semantics, opening a FIFO in read-only mode (`cat < /tmp/pipe` or `read -r`) receives EOF as soon as a writer (`dotoolc`) closes its write descriptor. Subsequent writers then fail because no reader is actively listening.

To maintain a continuous typing sink across multiple chunk submissions:
- The listener opens the FIFO in `O_RDWR` mode:
  ```python
  fd = os.open("/tmp/dotool-pipe", os.O_RDWR)
  with open(fd, "r") as f:
      for line in f:
          sys.stdout.write(line)
          sys.stdout.flush()
  ```
- Holding an `O_RDWR` descriptor keeps the write refcount $\ge 1$, preventing premature EOF and allowing sequential `dotoolc` invocations to submit commands uninterrupted.

---

## 3. Process Lifecycle & Teardown Safety

In multi-process test pipelines (`voxi eager` spawning `arecord` and `crispasr` child processes), sending `kill -TERM` to the parent process without terminating the child process tree leaves audio pipe descriptors open, causing shell `wait` commands to block indefinitely.

**Remedies applied:**
1. Process group signaling: using `pkill -TERM arecord` and explicit process group termination.
2. Bounded session runtime: limiting test execution with explicit timeouts (`sleep 2.5`).
3. Clean exit code handling: avoiding shell aborts on expected termination signals (`kill -TERM ... || true`).

---

## 4. Verification & Performance

Running `make test-e2e` executes in **~8.0 seconds** total:

```text
==> Running headless end-to-end integration test in container...
==> 1. Initializing isolated virtual audio daemon (PulseAudio)...
Server Name: pulseaudio
Default Sink: auto_null
Default Source: auto_null.monitor
==> 2. Initializing isolated dotool typing sink...
✅ Audio & Typing infrastructure ready.
==> 3. Running eager dictation pipeline against fixture (short-one-two.wav)...
  -> Playing /workspace/test/fixtures/short-one-two.wav into virtual microphone...
==> 4. Verifying pipeline outputs and deterministic transcription...
--- Voxi Chunks ---
INDEX   TIMESTAMP            AUDIO   RTF      RMS  LEVEL         STATUS      TRANSCRIPT
#1      2026-09-11 13:33:14  1.3s    0.35     305  [⣀⣀⣀⣀⣀⣠⣦⣀⣀⣀]  accepted    One.
#2      2026-09-11 13:33:15  1.1s    0.36     311  [⣀⣀⣀⣀⣠⣦⣀⣀⣀⣀]  accepted    Two.
-------------------
✅ Verified: Virtual audio streamed, VAD segmented utterances, CrispASR neural network transcribed deterministic output, and ring buffer recorded chunks!
==> Headless end-to-end integration test suite PASSED successfully!
```
