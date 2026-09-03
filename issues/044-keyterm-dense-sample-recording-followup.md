# 044: Record a Keyterm-Dense Dev Sample Corpus (Follow-up)

**Status**: Done — see [032 Section 7.2](032-small-en-project-vocabulary-biasing.md#72-keyterm-dense-corpus-result-2026-09-02-via-issue-044--gate-closed)
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Follow-up
**Related**: [032 small.en vocabulary biasing](032-small-en-project-vocabulary-biasing.md) (Section 7 measurement gate), [042 private dev sample recorder](042-private-dev-sample-recorder.md) (Section 7 first real-mic data point)

---

## 1. Problem & Motivation

Issue 042 shipped `voxi feedback sample record` and proved the workflow
end-to-end with three ad hoc real-microphone samples. Their aggregate WER
improved with prompting (0.40 → 0.10), but none of the three phrases happened
to contain an exact static-vocabulary keyterm, so `keyterm_recall` read 0/0 —
not useful signal for issue 032's still-open Section 7 measurement gate,
which specifically needs a corpus that exercises the technical vocabulary.

This ticket is a **follow-up reminder with the exact phrases to dictate**,
not new code. No implementation needed — just running `voxi feedback sample
record <name>` once per phrase below.

## 2. Proposed Phrases to Dictate

Say each phrase naturally (don't spell it out), then enter the corrected
transcript exactly as shown when prompted. These mirror
`testdata/speech-context/corpus.tsv`'s style and the static vocabulary in
`spec/models.yaml`'s `speech_context.terms`
(Voxi, voxtype, dotool, PipeWire, Wayland, systemd, Cobra, Golang, YAML,
JSON, Git):

```sh
voxi feedback sample record kt-core
# say: "Voxi uses voxtype with dotool on PipeWire and Wayland."

voxi feedback sample record kt-daemon
# say: "The systemd user service restarts the voxi-agent daemon."

voxi feedback sample record kt-cli
# say: "I wrote this CLI in Golang using Cobra for the command tree."

voxi feedback sample record kt-config
# say: "Update models.yaml, then run go test on context_test.go."

voxi feedback sample record kt-formats
# say: "The API returns JSON, but the spec files are YAML."

voxi feedback sample record kt-git
# say: "Commit this with Git and push to the default branch."

voxi feedback sample record kt-mixed-1
# say: "Push the PipeWire fix, then restart the Wayland session."

voxi feedback sample record kt-mixed-2
# say: "Voxtype's initial prompt biases small-dot-e-n toward Voxi's vocabulary."

voxi feedback sample record kt-ordinary-1
# say: an unrelated ordinary sentence with no technical terms, e.g. "I'll grab
# coffee before the next meeting starts."

voxi feedback sample record kt-ordinary-2
# say: another ordinary sentence, e.g. "The weather looks better this
# afternoon than it did this morning."
```

The two "ordinary" phrases matter as much as the technical ones: issue 032's
gate explicitly requires no material general-WER regression, not just a
keyterm-recall gain, so the corpus needs non-technical control phrases too.

## 3. Follow-up Steps

1. Record all phrases above (or a representative subset — more than three,
   enough that `keyterms_found`/`keyterms_total` in the bench output is
   non-zero for most fixtures).
2. Run:
   ```sh
   go run ./scripts/speech_context_bench -corpus ~/.config/voxi/samples
   ```
3. Paste the aggregate `summary` block into issue 032's Section 7.
4. Close issue 032's Section 7 gate (opt-in vs. default-on decision) based on
   the result, or note what's still missing.

## 4. Acceptance Criteria

- [x] At least the technical phrases (kt-core through kt-mixed-2) and both
  ordinary control phrases are recorded. **Done 2026-09-02** — all 10
  phrases recorded.
- [x] Bench output shows non-zero `keyterms_total` for the technical
  fixtures. **Done** — required manually populating `corpus.tsv`'s keyterms
  column (see 032 Section 7.2); the recorder does not infer keyterms from
  text automatically.
- [x] Aggregate WER/keyterm-recall/latency numbers are recorded in issue 032
  Section 7. **Done** — see
  [Section 7.2](032-small-en-project-vocabulary-biasing.md#72-keyterm-dense-corpus-result-2026-09-02-via-issue-044--gate-closed).

## 5. Non-goals

- No code changes — issue 042 already implemented everything needed.
- No automatic phrase generation or corpus curation tooling.

## 6. Result (2026-09-02)

Gate closed: keyterm recall 0.35 → 0.90, mean WER 0.259 → 0.095, latency
delta ~30 ms, zero repeats in either mode. Two data-quality issues found
along the way, both fixed by hand rather than as code changes (out of scope
for this ticket, but worth a future look if sample recording continues):
`corpus.tsv` rows can end up missing the trailing tab for an empty keyterms
field, and the recorder never prompts for or infers keyterms at all — every
row starts with an empty keyterms column regardless of content.
