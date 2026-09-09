# 099 — Replace corpus.tsv Sample Manifest With a More Robust Storage Format

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Dev Tooling / Data Format
**Related**: [098 unify chunks/sample lister](098-unify-chunks-list-and-feedback-sample-list-behind-a-shared-lister-add-all-full-short-to-sample-list.md)
(just landed, touches the same read path — sequence after or alongside this ticket, not
strictly blocking),
[097 external speech sample catalog](097-external-public-domain-speech-sample-catalog-download-on-demand-not-committed.md)
(a third, forward-looking sample source; any replacement format should keep it in mind),
[internal/devsample/sample.go](../internal/devsample/sample.go) (`ParseManifest`,
`FormatManifest`, `LoadManifest(In)`, `SaveManifest(In)`, `manifestFile = "corpus.tsv"`),
[internal/devsample/flow.go](../internal/devsample/flow.go) (`record`/`save-chunk`/`save-last`/
`list`/`remove`/`promote` call sites)

---

## 1. Problem & Motivation

Verbatim user request:

> move away from samples TSV and use a more robust way to store the data

The dev/feedback sample corpus is stored as a hand-parsed, tab-separated `corpus.tsv` manifest
in two places:

- the private per-user store, `~/.config/voxi/samples/corpus.tsv`, managed by
  `voxi feedback sample record/save-chunk/save-last/list/remove`;
- the public git-tracked corpus, `testdata/noise-samples/corpus.tsv`, populated by
  `voxi feedback sample promote`.

While manually editing `corpus.tsv` this session to correct an entry's transcript field, an edit
that removed a line's trailing tab character (used to represent an empty 4th/keyterms field)
silently produced a `malformed sample manifest line` error (`internal/devsample/sample.go:137`)
from `voxi feedback sample promote` — the format is fragile to hand-editing, and the error only
surfaces at promote/load time, not at write time. Recovered by rewriting the line with an
explicit trailing tab via `python3`.

## 2. Current Format

`internal/devsample/sample.go`'s `ParseManifest`/`FormatManifest`:

- Data rows: `<name>\t<wav file>\t<text>\t<keyterms>`, keyterms `|`-separated, 4th field optional
  (line split via `strings.SplitN(line, "\t", 4)`, `len(fields) < 3` is the only structural
  validation).
- Each data row is preceded by an interleaved `#ts <name> <RFC3339>` comment line carrying the
  sample's timestamp out-of-band from the data row itself (`ParseManifest` builds a
  `name -> time.Time` side map from these before attaching them to samples).
- Free-text `#`-prefixed comment lines are allowed at the top of the file and are format
  documentation (`FormatManifest` writes 4 fixed header comment lines) as well as
  `scripts/speech_context_bench`-compatibility notes.
- `sanitizeText` (sample.go:174) strips tabs/newlines from `text`/`keyterms` on write to protect
  the format, i.e. the format cannot represent a literal tab or newline in a transcript at all —
  a real speech transcript containing an em-dash-style pause or a multi-line correction would
  silently lose that structure.
- Two independent copies of this reader/writer pair exist implicitly: `SaveManifest`
  (`0o700`/`0o600`, private) and `SaveManifestIn` (`0o755`/`0o644`, public/git-tracked) both funnel
  through the same `saveManifestIn` — worth confirming any replacement format keeps this
  private-vs-public permission distinction and the atomic-write behavior it currently has.

This interleaved comment-per-row-plus-tab-sensitivity-plus-optional-trailing-field convention is
what makes the format error-prone by hand. Also worth checking during implementation: whether the
same brittleness bites the program's own writer path under concurrent writes, or a hypothetical
future entry containing characters `sanitizeText` doesn't anticipate.

Two real entries added this session are useful as concrete round-trip test cases for a
replacement: `bg-voice-aye-you-did-that` and `bg-voice-hospital-hallucination` in
`testdata/noise-samples/corpus.tsv` (commit `aa041b3`).

## 3. Open Questions (not prescribed — for the implementer to resolve)

- **Replacement format**: JSON Lines (one `Sample` object per line, append-friendly, diff-friendly,
  no interleaved-comment indirection)? A directory of per-sample sidecar files (e.g.
  `<name>.json` next to `<name>.wav`, self-describing, trivially hand-editable one sample at a
  time, no whole-manifest rewrite needed for a single edit)? SQLite (queryable, but adds a binary
  dependency and loses git-diffability for the public/git-tracked corpus)? A single JSON array
  file (simpler than JSONL but loses append-friendliness and line-level diffs)?
- **git-diffability of the public corpus matters**: `testdata/noise-samples/corpus.tsv` is
  committed and reviewed via `git diff`; whatever replaces it for the public corpus should stay
  reasonably diff-friendly (JSONL and sidecar files are strong here; a single large JSON array or
  SQLite file are weak).
- **Migration**: does this ship with a one-time converter (`voxi feedback sample migrate`?) that
  reads the existing `corpus.tsv` in both known locations and rewrites it in the new format, or is
  it a manual/documented step? Existing private and public corpora (8 and 24 samples respectively,
  as of this session) need to survive the transition.
- **Backward/forward compat with `scripts/speech_context_bench`**: `FormatManifest`'s current
  header comment explicitly documents `corpus.tsv`-compatibility with
  `scripts/speech_context_bench -corpus`. Does that script need updating in lockstep, or does it
  stay pointed at a legacy TSV export?
- **Sequencing with 098**: issue 098 (just implemented) added `internal/listing` and
  `internal/feedback/samplelist.go` reading `Sample` via `LoadManifest`/`LoadManifestIn`. A format
  change should keep the `Sample` struct's public shape (or update call sites) so 098's lister
  keeps working — check whether to do this before, after, or interleaved with any further 098
  follow-ups.
- **Validation at write time, not just read time**: whatever format is chosen, prefer one where a
  malformed hand-edit is either impossible (structured format with a real parser/validator) or
  fails immediately and clearly, rather than the current behavior of a cryptic error surfacing
  later at `promote`/`list` time.

## 4. Acceptance Criteria (indicative — format itself is an open question above)

- `corpus.tsv`'s interleaved-comment, tab-sensitive, hand-editable-but-fragile format is replaced
  by a structured format with real parsing/validation.
- Existing private (`~/.config/voxi/samples`) and public (`testdata/noise-samples`) corpora are
  migrated without data loss (name, WAV file reference, text, keyterms, timestamp all preserved).
- The private-vs-public permission distinction (`0o600` private / `0o644` public) and atomic-write
  behavior current `SaveManifest`/`SaveManifestIn` provide are preserved.
- `go test ./...` passes, including round-trip tests for the two real entries added this session
  (`bg-voice-aye-you-did-that`, `bg-voice-hospital-hallucination`).
- `voxi feedback sample record/save-chunk/save-last/list/remove/promote` all continue to work
  against the new format.
