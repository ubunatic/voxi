# 168 — Merge chunks/samples into longer clean voice material

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: issues 098, 165, 166, 167, internal/chunks/chunks.go (`Chunk`, `Buffer.Add`), internal/devsample/flow.go (`SaveChunkAsSample`), internal/audio (`WriteWAVAudio`), docs/TTSReading.md (VoxCPM presets)

---

## 1. Problem & Motivation

Eager streaming cuts dictation into sentence-sized chunks, but voice cloning and training
(VoxCPM reference presets, Piper datasets) and some dev/test work prefer longer, coherent, clean
material. Today a user speaking one paragraph ends up with several short chunks or samples that
cannot be joined. Question to answer first: can we merge them **safely**, meaning the merged WAV and its
transcript still match exactly and provenance stays traceable.

/goal Decide whether and how to add a safe `merge`/`combine` for chunks and for samples (for example `voxi chunks merge A B C NAME` and `voxi feedback sample merge NAME A B C`), and if the answer is yes, implement it with tests and `make install`; or stop and report when blocked on a user decision or denied permission.

## 2. Findings So Far (verify against live code)

- Chunks carry `Index`, `Timestamp`, `SessionID`, `AudioDurationSecs`, `PCMBytes`, transcripts and a WAV file (16 kHz mono via `audio.WriteWAVAudio`); this is enough to detect "adjacent in the same session" and to compute the real gap between two chunks.
- Samples live in `~/.config/voxi/samples/` as WAV plus a `corpus.tsv` row (id, wav, expected transcript, keyterms). Merging needs a new row with the joined transcript and the union of keyterms.
- Chunks are memory-backed and disappear on reboot; a merge should be able to work directly from chunks and write a sample.

## 3. Safety Questions To Resolve (record answers here, do not guess)

1. **Audio join:** identical format required (rate, channels, bit depth), else refuse. Insert a short measured or configurable silence gap instead of butt-joining; optionally cap it to the real recorded gap. No resampling in v1.
2. **Adjacency guard:** refuse chunks from different sessions or with a large time gap unless `--force`; refuse rejected chunks (`low_energy_transient`, `unvoiced_transient`, empty) unless `--force`.
3. **Transcript join:** joined text must equal what is spoken. Use single-space join and open the merged text in `$VISUAL`/`$EDITOR` (issue 166) for review before writing. Corrected sample transcripts win over ASR text.
4. **Consistency:** warn on big loudness mismatch between parts; no normalization in v1.
5. **Provenance and safety of originals:** never modify or delete inputs; record source ids/indices in the new entry (a manifest field, or a comment kept compatible with corpus.tsv readers). Refuse to overwrite an existing name without `--force`.
6. **Limits:** maximum merged duration suited to VoxCPM reference presets and Piper training; find the real limits before choosing (assumption unverified).
7. **Order:** default order is chunk index / sample timestamp; explicit argument order overrides.
8. **Atomicity:** write WAV to temp then rename; manifest update last, as `SaveChunkAsSample` does.

## 4. Milestones

M1 (design and decision): answer §3 from live code, choose command shapes and record the decision here; if any item cannot be made safe, close as "not worth it" with the reason.
M2 (implementation, only if M1 says yes): merge for chunks then samples, tests (format mismatch, cross-session refusal, order, gap, overwrite refusal, provenance), docs and `--help`, `make install`.

## Note from 169 (2026-09-29)

Command shape decided: `voxi sample merge ID --from A B C` (samples) or `--chunks` (chunk indices), writing a new sample whose sidecar `source` records the inputs; builds on 170/172. See docs/SampleStore.md §5.
