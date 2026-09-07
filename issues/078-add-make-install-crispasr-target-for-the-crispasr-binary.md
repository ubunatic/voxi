# 078 — Add make install-crispasr Target for the crispasr Binary

**Status**: Closed
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Enhancement
**Related**: [066 Cohere/Nemotron canary](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md), [074 Cohere backend integration](074-wire-cohere-transcribe-in-as-an-additional-selectable-asr-backend.md), [077 voxtype-optional Cohere path](077-make-voxtype-optional-and-remove-whisper-only-engine-assumptions.md)

---

## 1. Problem & Motivation

Cohere Transcribe (`cohere-transcribe`) is voxi's default eager ASR engine
(issues 074/076), driven through a separate binary called `crispasr`
(`internal/eager/cohere.go`: `crispasrBinary = "crispasr"`, resolved via
`LookPath("crispasr")`). Issue 077 fixed voxi's own code so it no longer
*requires* `voxtype` before reaching the Cohere path — but `crispasr` itself
still has no installation story anywhere in the project. On a fresh dev
machine, `crispasr` is simply absent, and there is currently no Makefile
target or setup script that installs it, which just blocked a live
end-to-end Cohere dictation test in the same session that closed 077.

Issue 066 (canary, closed) already proved `crispasr` works, but only via a
manual reproducible from-source build:

```
git clone https://github.com/CrispStrobe/CrispASR --recursive --depth 1
cmake -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build -j$(nproc)
```

See issue 066 §7.2 for the exact commands. This build takes roughly 7 minutes
on that hardware, pulls down a roughly 470MB source tree, and compiles 119
unrelated ASR/TTS backends alongside the one voxi actually needs — even
though the resulting `crispasr` binary itself is only 4.7MB (MIT licensed).
This is a heavyweight, slow default install path for something voxi only
needs as a single binary.

Note: `crispasr`'s own runtime weight download (the roughly 1.66 GiB GGUF
model cache) is already handled automatically at runtime by
`ensureCohereWeights` in `internal/eager/cohere.go` and is explicitly out of
scope here — this issue is only about installing the `crispasr` *binary*.

## 2. Findings and Required Changes

No prebuilt-binary or packaging investigation has been done yet for this
project's own use — issue 066 only validated the from-source build path.
Implementation must establish, in this priority order, which install path is
actually available before writing `make install-crispasr`:

1. **Preferred — prebuilt upstream release binaries.** Check whether
   https://github.com/CrispStrobe/CrispASR publishes GitHub Releases with
   prebuilt binaries per architecture (e.g. via `gh release list` / the
   GitHub releases API). If so, `install-crispasr` should download the
   official binary matching the current architecture (`uname -m`, e.g.
   `x86_64`/`aarch64`) directly — no local build required.
2. **Fallback — native OS packaging.** If no prebuilt releases exist, check
   whether `crispasr`/CrispASR is packaged for Debian or Fedora (`apt-cache
   search`, `dnf search`, Debian/Fedora repo search, or a `.deb`/`.rpm`
   attached to an upstream release) and install via the native package
   manager if so.
3. **Last resort — from-source cmake build.** Only if neither (1) nor (2) is
   available, fall back to issue 066 §7.2's reproducible cmake build. If this
   is the path implementation ends up taking, explicitly flag the ~7 minute
   build time and the 119 unrelated backends pulled in as a real, documented
   cost in this ticket and in `make help`/target output — do not silently
   default to this path without recording why the faster options were
   unavailable.

Follow the project's existing Makefile conventions (phony sentinel pattern,
self-documenting help line — see `docs/Make.md`) and match the style of
existing targets surfaced by `make help`: `preflight`, `build`, `build-all`,
`install`, `install-all`, `install-system`, `install-modifierd`,
`install-user-services`, `restart-service`.

`install-crispasr` should be an explicit, standalone opt-in target for now —
`crispasr` is only needed for the Cohere engine path, and voxtype-only setups
should not be forced to pull it in. Whether it later belongs in
`install-all` is an open decision left to implementation to make and justify
in this ticket or a follow-up, not decided here.

## 3. Acceptance Criteria

- `make install-crispasr` exists, is documented in `make help` output
  following the project's self-documenting help-line convention, and is
  guarded by the standard phony sentinel pattern (`docs/Make.md`).
- Verify at implementation time and record the answer in this ticket:
  does CrispASR upstream publish GitHub release binaries for
  linux-x86_64/linux-arm64 (or equivalent)? If yes, `install-crispasr` uses
  them, selecting the binary matching the current architecture.
- If no prebuilt releases exist, verify and record whether `crispasr`/CrispASR
  is available via apt (Debian) or dnf (Fedora), or as a `.deb`/`.rpm` in an
  upstream release, and use that path if so.
- If neither prebuilt binaries nor OS packages exist, `install-crispasr` falls
  back to the issue 066 §7.2 cmake build, and this ticket documents that
  choice along with the build-time/dependency-footprint cost it carries.
- Running `make install-crispasr` on a machine without `crispasr` results in
  a working `crispasr` binary on `$PATH` (or in voxi's expected install
  location), verified by a successful invocation (e.g. `crispasr --version`
  or equivalent) after the target completes.
- `install-crispasr` is NOT added to `install-all` unless this ticket (or a
  documented follow-up) explicitly justifies doing so.
- No changes to `internal/eager/cohere.go`'s existing weight-download logic
  (`ensureCohereWeights`) — that remains out of scope and untouched.

## 4. Verification and Delivery

1. Investigate upstream release/packaging availability first (per §2's
   priority order) before writing any Makefile logic, and record findings
   directly in this ticket.
2. Implement the winning install path as `make install-crispasr`, following
   `docs/Canary.md`'s probe-before-build practice for the external download
   mechanism chosen.
3. Run the target on a machine without `crispasr` installed and confirm a
   working binary results.
4. Run `make check` / `go test ./...` to confirm no regressions to unrelated
   Makefile targets.
5. This ticket does not require a live Cohere dictation test on its own —
   that verification belongs to issue 077's already-closed scope and any
   future end-to-end retest — but installing the binary via this target
   should be confirmed to unblock that path.

## 5. Non-Goals

- Changing or touching `internal/eager/cohere.go`'s Cohere weight-download
  logic (`ensureCohereWeights`) in any way.
- Adding `install-crispasr` to `install-all` by default.
- Building or maintaining voxi's own CrispASR fork, packaging, or release
  pipeline — this only consumes whatever upstream already publishes.
- Re-litigating issue 066's canary conclusion to adopt Cohere Transcribe, or
  issue 077's voxtype-optional scope.

## 6. Resolution (2026-09-07)

**Investigation result**: upstream `CrispStrobe/CrispASR` *does* publish prebuilt
GitHub Release binaries for Linux, per architecture — confirmed via
`gh release view v0.8.32 --repo CrispStrobe/CrispASR --json assets`. The plain
CPU builds are `crispasr-linux-x86_64.tar.gz` and `crispasr-linux-arm64.tar.gz`
(alongside cuda/vulkan/hip/avx512/legacy variants not needed here). §2's
priority-1 path applies — no OS packaging check or from-source build was
needed.

`make install-crispasr` downloads the architecture-matching tarball from
`https://github.com/CrispStrobe/CrispASR/releases/latest/download/<asset>`
(a stable, no-API-call redirect URL, canary-verified with `curl -sIL`),
extracts it to `~/.local/lib/voxi/crispasr/` (binary + its bundled
`.so` runtime deps: libopenblas, libgomp, libgfortran, libquadmath,
libc2pa_c — the release binary has `RUNPATH=$ORIGIN`, verified with
`readelf -d`, so these must stay siblings of the binary; a bare copy without
them fails with `error while loading shared libraries`), and symlinks
`crispasr` into `~/go/bin` (the same install location `make install` uses
for `voxi`, confirmed by `ldd`/manual run that `$ORIGIN` resolves through
the symlink to the real directory).

**Verification**:
- `make install-crispasr` ran for real on this machine (no prior `crispasr`
  on `PATH`): downloaded 37.55 MiB, `which crispasr` → `~/go/bin/crispasr`,
  `crispasr --version`/`--help` run successfully.
- `make check` (`go vet`, `go test ./...`, spec validation) — all packages
  pass, no regressions.
- `make restart-service` picked up the change; `voxi record status` returns
  `idle` without touching `voxtype` (issue 077's fix intact).
- Live functional proof of the Cohere path: ran the exact binary/args/weights
  the daemon uses (`crispasr -m ~/.cache/voxi/models/cohere-transcribe-q5_0.gguf
  --backend cohere -t 6 --language en -np -nt -f jfk.wav`) against
  whisper.cpp's real JFK reference clip, freshly downloaded — produced a
  correct transcription ("And so, my fellow Americans, ask not what your
  country can do for you, ask what you can do for your country.") in 2.2s.
  This proves the crispasr binary + downloaded Cohere weights transcribe real
  speech correctly now that this ticket's gap is closed. A full mic-to-keystroke
  `voxi record toggle` pass requires a human physically speaking into this
  session's hardware mic, which an agent session cannot trigger itself — not
  re-verified here beyond this direct-invocation proof.
- `install-crispasr` intentionally left out of `install-all` per §3/§5 — no
  reason found to change that default.

Implementation: `Makefile` (`install-crispasr` target only; no changes to
`internal/eager/cohere.go`).
