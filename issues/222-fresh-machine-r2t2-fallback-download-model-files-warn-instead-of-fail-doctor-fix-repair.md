# 222 — Fresh-machine R2T2 fallback: download model files, warn instead of fail, doctor --fix repair

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Bug
**Related**: 181 (lmcoder owns the R2T2 model), 184 (R2T2 as default without cleanup), 211 (doctor --fix starts lmcoder ASR with cost notice), 182 (model file cleanup)

/goal On a fresh machine with `r2t2-confucius4-voxi` selected, `voxi install` either gets the R2T2 fallback running (after a confirmed download with sha256 check) or warns and leaves the unit disabled, and never fails; `voxi doctor --fix` repairs the same state. Verified on a machine without `~/.cache/voxi/models`. Stop and report when blocked on a user decision or denied permission. On a loop or hard blocker, clear the goal and write a failure report here.

---

## 1. Problem & Motivation

On a fresh machine, `voxi install` cannot make the R2T2 fallback work:

- `internal/install/install.go:243-257` returns a hard error when the selected ASR model is
  `r2t2-confucius4-voxi` and its model files or `llama-server` are missing. Install fails, which
  breaks the rule that optional model and hardware steps never fail install.
- Nothing downloads the two R2T2 files. Issue 181 records that they were re-downloaded by hand on
  2026-10-06.
- `internal/doctor/asr.go:293-296` gives guidance only for this fallback. `voxi doctor --fix`
  cannot repair it.

Observed on 2026-11-12 on a new Intel machine: the config selects `r2t2-confucius4-voxi`, the
model directory holds only `cohere-transcribe-q5_0.gguf`, `voxi-r2t2.service` is installed and
disabled, `llama-server` is not on `PATH`, and dictation chunk #49 failed with connection refused.

## 2. Technical Specification / Findings

Verified 2026-11-12:

- Both files are public and anonymous downloads work (`curl -sIL` on
  `https://huggingface.co/netease-youdao/Confucius4-R2T2-GGUF/resolve/main/<file>` returns HTTP 200).
- `Confucius4-R2T2-Q4_K_M.gguf`: 1107404736 bytes, sha256 `fa3cb46c8c3a66a58812b9098ba6e96a0266d4e8c9b3cf5ba34432fd2f9f6466`.
- `mmproj-Confucius4-R2T2-Q8_0.gguf`: 348336544 bytes, sha256 `8dc2c67e6a0484114928142d098db7ad94ae9f34c78948ef9d37a9678418cb65`.
- Total is about 1.4 GiB, not 1 GB. Both hashes match the prefixes and suffixes in issue 181.
- The unit `~/.config/systemd/user/voxi-r2t2.service` runs `/usr/bin/llama-server` with `--port 18131`.
  That path is a placeholder: `llama-server` is not on `PATH` on this machine and I have not checked
  `/usr/bin` directly.

Decisions (advisor review, smart model, 2026-11-12, with user `/decide` authority):

- **Missing files**: offer a confirmed download; if declined or failed, warn and leave the unit
  disabled. No hard failure.
- **Confirmation**: y/N showing the size, default no when non-interactive. `doctor --fix` offers the
  same download.
- **llama-server**: hint and check only; no package-manager step for now (package names differ by
  distro, and the repo already honours `llama_server_path`).

Open facts (verify before implementing):

- Source of port 8749. Chunk #49 went to `127.0.0.1:8749` with model `r2t2-confucius4`. Neither
  `spec/models.yaml` (primary entry uses lmcoder proxy `:8735`) nor `~/.config/voxi/config.yaml`
  (`openai_asr_base_url: http://127.0.0.1:8090/v1`) explains it.
- Whether a per-model `base_url` overrides the global `openai_asr_base_url`. The comment at
  `spec/models.yaml:147` says pinned models do not share it; not tested.
- Whether `llama-server` on Arch is available as a package, and its name.

## 3. Implementation & Verification Plan

1. **Spec**: put the repo, file names, sizes and full sha256 for both files in `spec/models.yaml`
   (Spec rule: values not duplicated in Go).
2. **Download**: reuse the resilient HTTP helper (`internal/install/install.go:957`). Write to a
   `.part` file, verify size and sha256, then rename. Never accept a file that only passes
   `IsRegular`.
3. **Install**: replace the hard error at `install.go:243-257` with the confirm, download, warn
   flow above. Keep `voxi-r2t2.service` disabled unless both files verify and `llama-server` resolves.
4. **Doctor**: add a check for "files present but unit disabled" and "files missing"; `--fix` offers
   the same download and enable. Keep the rule that the unit must not start before its files are
   checked.
5. **Coordinate with 181**: 181 says lmcoder's registry should own the download. Record in 181
   whether voxi's downloader replaces that plan or only covers the no-lmcoder case, before merging.
6. **Tests**: installer tests with fake `Effects`, as in `internal/install/hardware_test.go`: files
   missing and declined; download with bad hash rejected; files present; `llama-server` missing.
7. **Docs**: update `docs/InstallationArchitecture.md` and `docs/ASREngines.md`.

Verification on the live host: remove nothing; test on a copy of `~/.cache/voxi/models` in a
temporary `XDG_CACHE_HOME` before touching the real directory.
