# 169 — Discovery: one sample concept and store (chunks, samples, feedback, training vocabulary)

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Architecture
**Related**: 099 (replace corpus.tsv), 119 (consolidate local config storage), 097 (external sample catalog), 098 (shared lister), 117 (sample import), 042/055 (recorder, save-chunk), 151/153 (voice training, allowlist), 165/167/168 (chunks delete, filters, merge), 166 ($EDITOR transcripts), docs/ChunkDiagnostics.md, docs/TTSReading.md

**Executor**: Opus (`claude-opus-5-5`). This is a research ticket for the strongest model, not for a low-cost developer.

---

## 1. Problem & Motivation

The CLI and docs use overlapping words for audio material and its handling: **chunk**, **sample**,
**feedback**, **training**, **corpus**, **voice**, **clone**, **promote**, **import**, **record**, and the `noise-samples` and
`speech-context` test dirs. Users (and agents) cannot tell them apart. Observed examples:

- `voxi chunks` (ephemeral recordings in `/run/user/<uid>/voxi/chunks/`, gone on reboot, last 100) versus `voxi feedback sample` (persistent, in `~/.config/voxi/samples/`, with `corpus.tsv`). A chunk becomes a sample via `save-chunk`; `promote` means something else (private noise sample to public git-tracked corpus); `voxi voice clone` takes yet another list (`voice-training.txt` allowlist).
- Samples serve different purposes with different safety rules but share one store: dictation test material, keyboard-clack and other noise samples used to check that no speech is detected, keyterm-dense dev samples, and the user's own clean voice for cloning/training. Development samples must never end up in voice training; today that separation is one allowlist file.
- Storage is scattered: private samples, public `testdata/noise-samples` and `testdata/speech-context`, VoxCPM presets under `~/.local/share/voxi/voxcpm/voices/` and `spec/tts.yaml`, Piper datasets and voices under `~/.local/share/voxi/voices/`.

Direction proposed by the user (to test, not to assume): **one store** for all samples under the config
folder, with the sample's **purpose** recorded per sample (subfolders and/or metadata in a database),
strict separation between purposes, and a **minimal vocabulary** on the command line where "a sample is a sample".

/goal Produce a decision document and a staged implementation plan for a single sample concept, store and small command vocabulary, with a clean purpose separation that makes it impossible to train on development samples by accident; file follow-up tickets for the implementation; or stop and report when blocked on a user decision or denied permission.

## 2. Questions To Answer (record findings in the ticket, verify against live code first)

1. **Inventory:** every place audio and transcripts are stored or named (dirs, files, manifests, allowlists, spec entries, docs, man pages, website, CLI commands and flags, Go package names). Which words mean what today?
2. **Vocabulary:** propose the smallest command set. Candidates: a top-level `voxi sample` (list, show, play, record, add, edit, delete, merge, import) with chunks folded in as "recent recordings" or as a `--recent` source; `feedback` limited to stop words, replacements, vocabulary. Decide what happens to the `chunks`, `feedback sample` and `voice clone/prepare/train` names (aliases, deprecation period).
3. **Purposes and separation:** define the purpose set (for example `dictation`, `noise`, `voice`, `dev`), whether purpose is a folder, a metadata field, or both, and the invariants (voice cloning and training read only `voice`; noise-detection tests read `noise`; public corpus contains only samples marked shareable). One sample, one primary purpose, or tags?
4. **Storage:** single root under `~/.config/voxi/` (or XDG data dir, since audio is data and not config); fate of `corpus.tsv` (see 099: SQLite, JSONL, or per-sample sidecar files) and of `voice-training.txt`; how VoxCPM presets and Piper datasets reference samples instead of copying them.
5. **Lifecycle:** chunk (ephemeral) to sample (persistent) step, transcript correction ($EDITOR, 166), merge (168), delete filters (167), import/export between machines (117), public promotion (privacy rules, only noise/shareable).
6. **Migration:** how existing `~/.config/voxi/samples/` (31 files on the dev machine), `testdata/` and allowlists move without data loss; automatic, idempotent, reversible; tests use temp dirs only.
7. **Safety and privacy:** private-by-default permissions (0700/0600 today), consent wording for cloning only the user's own voice, and how the tool refuses wrong-purpose use.
8. **Docs and discoverability:** one glossary page and consistent help text; how agents find the right command (`voxi --help` wording, man pages, website).

## 3. Deliverables

- Decision record in `docs/` (glossary, purpose model, storage layout, command map, migration plan).
- Follow-up tickets (`M`-sized implementation tickets) filed by the executor, each linked here.
- Explicit list of things deliberately **not** changed.

## 4. Notes

Re-verify all statements above against live code and recent commits before starting; issues 099 and 119 may already cover part of the storage question, so merge rather than duplicate. Record open uncertainties instead of inventing details.
