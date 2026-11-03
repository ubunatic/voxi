# 219 — Jules report: how we run the untested TTS and STT models

**Status**: Closed — Jules report discarded: failed evidence check, no findings filed; manual process review still open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Documentation
**Related**: 200 (voxi-model evaluation skill), 199 (tested models catalog), 216, 217, 218 (untested models to evaluate)

---

/goal Produce a written report of how voxi evaluates untested TTS and STT models today (scripts, bench,
samples, docs, where verdicts are recorded), with checked `path:line` evidence and improvements, and
submit it as a pull request, or stop and report when blocked on a user decision or denied permission.

## 1. Problem & Motivation
Issues 216, 217 and 218 ask to try and benchmark three new models. The steps for that work are spread
across `docs/Canary.md`, `docs/BenchBaseline.md`, `docs/TestedModels.md`, `spec/models.yaml`,
`spec/tts.yaml` and the scripts. No single written description exists of the path an untested model
takes, so each evaluation starts from scratch.

## 2. Findings
Jules run jules-ideas-untested-model-evaluation (session 4376394707546715485, PR ubunatic/voxi#3) was
discarded: most of its path:line citations did not support their claims. PR closed, branch deleted.
No findings were filed. A checked write-up of the current path is still open.

## 3. Implementation & Verification Plan
Documentation only, to be written by the host with each claim checked against the code.

## Jules runs
| kind | session name | Jules ID | PR | state |
| --- | --- | --- | --- | --- |
| ideas | jules-ideas-untested-model-evaluation | 4376394707546715485 | ubunatic/voxi#3 | done: report discarded, PR closed, branch deleted |
