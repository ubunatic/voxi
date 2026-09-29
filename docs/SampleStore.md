# Sample Store: One Sample Concept, Purposes, Command Map

Decision record from issue 169 (2026-09-29). Status: **decided, not yet implemented**.
Implementation tickets: 170–175 (see §8).

## 1. Glossary

| Word | Meaning (after this decision) |
|---|---|
| **chunk** | One ephemeral recording cut by eager dictation, with its ASR diagnostics. Lives in `$XDG_RUNTIME_DIR/voxi/chunks/` (fallbacks in `internal/chunks.StorageDir`), last 100 kept, gone on reboot. Debug material, never a sample by itself. |
| **sample** | One persistent WAV/FLAC plus its metadata (expected transcript, keyterms, purpose, provenance). The only persistent audio unit. |
| **purpose** | The one thing a sample may be used for: `dictation`, `noise` or `voice` (§3). |
| **store** | The directory tree holding samples: the private store (per user) and the public store (git-tracked in the repo). |
| **publish** | Copy a sample from the private into the public store (today: `promote`). Only `noise`. |
| **import** | Merge samples from another machine's private store. |
| **voice** | An installed TTS voice (Piper `.onnx` or a VoxCPM/cloned reference WAV). Built *from* `voice` samples; not a sample itself. |
| **feedback** | Rules that change dictation output: stop words, replacements, silence artifacts, vocabulary. No audio. |
| **corpus** | Retired as a user-facing word. `corpus.tsv` is the legacy manifest format. |

## 2. Inventory (verified 2026-09-29 against commit 627b143)

Stores and files:

- Private samples: `~/.config/voxi/samples/` — 29 audio files + `corpus.tsv` + `voice-training.txt`
  (31 entries). Dir 0700, files 0600, except `calm-reference.wav` and `voice-training.txt`, which are **0644**.
- Allowlist: `voice-training.txt` (4 ids) — the only guard that keeps dev samples out of voice training
  (`internal/tts/clone/dataset.go`, `AllowlistFile`).
- Public noise samples: `testdata/noise-samples/` (FLAC, git-lfs, own `corpus.tsv` whose header comment
  still says "Private local dev samples"). Contains two `bg-voice-*` files marked as distant background speech.
- Speech-context bench: `testdata/speech-context/corpus.tsv` + README tracked, WAVs git-ignored and
  recorded locally (one local file: `artifact-keyboard-smash.wav`, a duplicate of a private sample).
- Voice training output: `~/.local/share/voxi/voice-training/{dataset,runs}` (copies of allowlisted WAVs).
- Installed voices: `~/.local/share/voxi/voices/` (Piper models + `cloned.wav` from `voice clone`).
- VoxCPM presets: `~/.local/share/voxi/voxcpm/voices/{full,short}.wav`, copied/trimmed from
  `voices/cloned.wav` by `voxi install` (`internal/install/voxcpm.go`), paths in `spec/tts.yaml`.
- Voice demos: `~/.local/share/voxi/voice-demo/` (TTS output, not samples).

Commands and packages:

- `voxi chunks list|show|play|delete` — `internal/chunks`.
- `voxi feedback sample list|play|record|remove|save-chunk|save-last|promote|import` —
  `internal/devsample` (store, capture, editor) and `internal/feedback` (command, list, import).
- `voxi feedback import` area `samples` — `internal/feedback/import.go`.
- `voxi voice prepare|train|clone` with `--samples-dir` — `internal/tts/clone`.
- Scripts reading `corpus.tsv`: `scripts/speech_context_bench`, `scripts/clack_features`.
- Docs/website mentioning these words: README.md, docs/{ChunkDiagnostics,TTSReading,VoiceInput,ASREngines,
  LLMTranscriptCleanup,LiveMicMeter}.md, website/{index,dev/index,man/index}.html.

Word collisions found: "sample" (dev sample, noise sample, voice sample, "samples" as audio frames in
`reference_start_sample`), "corpus" (private store, public store, bench fixture), "promote" (private to
public), "clone" (copy one sample to a voice profile), "training" (allowlist name and Piper runs),
"speech-context" (both an eager flag and a test dir).

## 3. Purpose Model

Three purposes, **one primary purpose per sample**, expressed as the **folder** the sample lives in.
No tags: a tag set would make "is this safe to train on?" a query over metadata instead of a path check.

| Purpose | Content | Read by | Publishable |
|---|---|---|---|
| `dictation` | Speech with an exact expected transcript and optional keyterms (today's dev and keyterm samples, short words) | ASR accuracy checks, bench scripts | never |
| `noise` | Audio that must yield no text (keyboard, mouse, motor, silence, distant background) | no-speech / artifact checks, `clack_features` | yes, after review |
| `voice` | The user's own clean, read speech for cloning and training | `voice prepare|train|clone`, VoxCPM presets, `sample merge` targets | never |

Invariants (enforced in code, tested):

1. `voice prepare|train|clone` read only `voice/`. There is no flag to point them at another purpose.
2. Adding to or moving into `voice/` requires the consent confirmation "this is my own voice"
   (interactive `y`, or `--own-voice` flag in scripts); the consent time is stored in the sidecar.
3. Publish accepts only `noise/` samples; the public store has only a `noise/` folder.
4. Dictation checks may read `voice/` as extra read-only material (one-directional; voice material is
   also clean dictation), never the other way round.
5. Changing purpose is an explicit `voxi sample move ID PURPOSE`; `voice` as a target triggers rule 2.

## 4. Storage Layout

Audio is data, not config, so the private store moves to the XDG data dir:

```
$XDG_DATA_HOME/voxi/samples/          (default ~/.local/share/voxi/samples, 0700)
  dictation/<id>.wav  <id>.json       (files 0600)
  noise/<id>.wav      <id>.json
  voice/<id>.wav      <id>.json
testdata/samples/noise/<id>.flac <id>.json   (public store, 0644, git-lfs)
```

- **Metadata:** one JSON sidecar per sample (`id`, `transcript`, `keyterms`, `created`, `source`
  = `record|chunk:<session>/<index>|merge:<ids>|import:<host>`, `consent` for voice). Chosen over
  SQLite (binary, poor diff, extra dep) and a single JSONL file (one corrupt write breaks the whole
  store, merge conflicts on import). Sidecars make import, delete and publish per-file atomic.
  This **supersedes issue 099**.
- **IDs** stay `^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$` and are unique across all purposes.
- `corpus.tsv` becomes an export format only (`voxi sample export --tsv`) for scripts that still
  want it; the scripts are switched to the store reader.
- `voice-training.txt` is retired: membership in `voice/` replaces it.
- **Derived files stay copies but record provenance:** Piper datasets, `voices/cloned.wav` and
  VoxCPM presets are caches regenerated from `voice/` samples. `config.yaml` records the sample id
  (`tts_voice_sample`) next to the existing `tts_voice_reference_wav`. No symlinks (engines and
  install copy with 0600; a symlink into the store would leak store paths into engine configs).
- Issue 119 (config formats): samples stay **out** of that consolidation; they are data, not config.

## 5. Command Map

```
voxi sample list [--purpose P] [--public] [--recent]   --recent lists chunks as candidates
voxi sample show ID
voxi sample play ID
voxi sample record [--purpose P] ID                    default purpose: dictation
voxi sample add ID --chunk N|--last [--purpose P]      replaces save-chunk / save-last
voxi sample edit ID                                    transcript in $VISUAL/$EDITOR (166)
voxi sample move ID PURPOSE                            consent gate for voice
voxi sample delete ID...                               replaces remove
voxi sample merge ID --from A B C                      samples or --chunks (168)
voxi sample import PATH                                other machine's store (117)
voxi sample export --tsv                               legacy manifest for scripts
voxi sample publish ID                                 noise only; replaces promote
```

Kept as they are: `voxi chunks ...` (diagnostics of ephemeral recordings, different lifetime and
fields; 165/167 filters stay there), `voxi voice prepare|train|clone` (they produce voices; only
`--samples-dir` becomes `--store`, and `clone --sample` keeps its name). `voxi feedback` keeps only
stop-word, replacement, silence-artifact, vocabulary, status, import (areas minus `samples`).

Deprecation: `voxi feedback sample <verb>` stays one release as a hidden alias that prints
`deprecated: use voxi sample <new verb>` on stderr and then runs the new command; removed in the
release after. `--samples-dir` likewise.

## 6. Lifecycle

```
mic --eager--> chunk (runtime, last 100) --sample add --chunk N--> sample (dictation|noise|voice)
mic --sample record------------------------------------------------^
chunks/samples --sample merge--> new sample (sources in sidecar; inputs untouched)
sample --sample edit--> corrected transcript     sample --sample move--> other purpose
noise sample --sample publish--> public store    store --sample import--> store
voice samples --voice prepare/train/clone--> voices (derived copies)
```

## 7. Migration

`voxi sample migrate` (also run automatically, once, by the first `voxi sample` call that finds the
legacy dir and no new store):

1. Read `~/.config/voxi/samples/corpus.tsv` and `voice-training.txt`.
2. Purpose assignment: allowlisted ids -> `voice`; ids whose transcript is empty or a bracketed
   `[...]` noise note -> `noise`; everything else -> `dictation`. Print the plan; `--dry-run` shows only.
3. Copy (never move) WAV + write sidecar per sample into the new store with 0700/0600, verify size
   and SHA-256, then write `~/.config/voxi/samples/MIGRATED` with the target path and timestamp.
   The legacy dir is left in place; deleting it is a manual user step.
4. Idempotent: existing target with same hash is skipped; different hash stops with an error.
5. Reversible: `voxi sample export --tsv --to DIR` rebuilds a legacy dir.
6. Public store: `testdata/noise-samples/` is converted in-repo with `git mv` to
   `testdata/samples/noise/` plus sidecars in one commit.
7. Tests use `t.TempDir()` only; nothing touches the real store.

## 8. Implementation Tickets

- 170 — Sample store package: layout, sidecars, purpose folders (supersedes 099)
- 171 — Migrate legacy sample dir and allowlist into the new store
- 172 — `voxi sample` command set and `feedback sample` deprecation aliases
- 173 — Purpose guards: voice-only training/cloning, consent, noise-only publish
- 174 — Public noise store review and move to `testdata/samples/noise`
- 175 — Glossary, help text, man pages and website wording

Order: 170 -> 171 -> 172 -> 173; 174 after 170; 175 last. 168 (merge) builds on 172.

## 9. Deliberately Not Changed

- `voxi chunks` name, storage, retention and its delete filters (165/167).
- `voxi voice` command names and the Piper/VoxCPM engine file locations.
- Feedback rule files (stop words, replacements, vocabulary) — that is issue 119.
- The `--speech-context` eager flag name.
- Audio format (16 kHz mono PCM WAV private, FLAC public).
- No database, no tags, no cloud sync.

## 10. Open Uncertainties

- Decided: the two public `bg-voice-*` noise files stay public (user, 2026-09-29). Do not ask again.
- Decided (user, 2026-09-29): `kt-sentences-plus-*` are noisy real-world speech for dev/testing, purpose `dictation`
  (speech must be detected). Imperfect dictation samples stay `dictation`. The migration heuristic may still
  misclassify others, e.g. `artifact-keyboard-smash`; the dry-run plan must be reviewed.
- Decided: no backup step is needed for the move to `$XDG_DATA_HOME` (user, 2026-09-29).
- VoxCPM `full` preset expects the exact reference text in `spec/tts.yaml`; linking it to a sample id
  assumes the sample transcript matches that text. Not verified for `calm-reference`.
