# Untested Model Evaluation

Target: how voxi evaluates untested TTS and STT models today.

## Current path

1. **Phase 1: Catalog & Prior Art Audit**
   - Check `docs/TestedModels.md` before testing to avoid duplicating past evaluations or re-testing rejected models. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:34)

2. **Phase 2: Feasibility & Guardrails Gate**
   - Ensure the runtime compiles to a native binary, GGUF/ggml, ONNX, or lightweight loopback server. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:36)
   - Perform a Direct Lean Extraction if possible, or fallback to a Containerized Spike. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:38)
   - Perform Disk Space Pre-flight & External Scratch Mount check. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:43)
   - Verify permissive open-source license (MIT, Apache-2.0, BSD). (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:49)
   - Specify CPU and/or Vulkan GPU expectations on `t14` / `x600`. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:50)

3. **Phase 3: Canary-First Probing**
   - Isolate model probe in `scripts/canary_<model>/` before touching core engine code. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:52)
   - Test CLI flags, stdout/stderr formatting, startup banner, and device detection. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:53)
   - Measure first-audio / first-token latency, RTF, and RSS memory. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:54)

4. **Phase 4: Engine Integration & Spec Wiring**
   - ASR: wire into `spec/models.yaml`. If a new engine, extract runner into `internal/<engine>` and delegate from `internal/eager`. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:56)
   - TTS: wire into `spec/tts.yaml`. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:57)

5. **Phase 5: Benchmark Runner & Verification**
   - ASR: Add model to `internal/bench` multi-engine runner (`voxi bench`). (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:59)
   - TTS: Run `scripts/canary_tts/benchmark.py` against reference sentence. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:60)
   - Pass `make test-q1`, update `docs/TestedModels.md` and baseline docs. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:61)
   - Execute `make restart-service` and verify live functionality. (path:`issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md`:62)

The verdict is recorded in `docs/TestedModels.md` (e.g. Adopted, Rejected, Parked) alongside hardware details and benchmark metrics, and for benchmark baselines in `docs/BenchBaseline.md`.

## Proposals

1. **Add explicit disk space checking and cleanup to canaries**
   *Why*: Canaries that download heavy model weights or run in containers can silently consume disk space, causing failures later. Issue 200 Phase 2 outlines a "Disk Space Pre-flight" but canary scripts (e.g., `scripts/canary_tts/benchmark.py`) lack standard checks or cleanup handlers for large temporary files.
   *Smallest step*: Introduce a standard bash/Python preamble for `scripts/canary_*` to `df -h` before large downloads and ensure `trap "rm -rf tmp_model_dir" EXIT` is used consistently.
   *Cite*: `scripts/canary_tts/benchmark.py`:14

2. **Automate BenchBaseline.md updates for new models**
   *Why*: When adding a new ASR model, developers must manually run `voxi bench --json out.json` and then format the table in `BenchBaseline.md`. This is error-prone and can lead to stale documentation.
   *Smallest step*: Add a simple tool (e.g. `scripts/update-bench-baseline.py`) that reads `out.json` and patches the Markdown table in `docs/BenchBaseline.md` directly.
   *Cite*: `docs/BenchBaseline.md`:49

3. **Standardize canary execution environment (e.g., systemd-run or cgroups) to prevent resource monopolization**
   *Why*: Model probes (especially local CPU/GPU-intensive tasks) can hang the user's primary desktop session or consume excessive RAM, making the machine unresponsive.
   *Smallest step*: Wrap canary execution in a standardized shell function that uses `systemd-run --user --scope -p MemoryMax=8G` to cap memory usage during exploratory runs.
   *Cite*: `docs/TestedModels.md`:46

4. **Extract model definitions to individual files in a directory rather than a single `spec/models.yaml`**
   *Why*: As more models are tested and onboarded, `spec/models.yaml` will grow and become unwieldy to maintain and review, especially with model-specific comments and stop words.
   *Smallest step*: Change the spec parser to read from `spec/models/*.yaml` instead of a single file, and migrate existing entries.
   *Cite*: `spec/models.yaml`:40

5. **Formalize the 'Sample Store' extraction for ASR accuracy benchmarking**
   *Why*: Currently, `voxi bench` uses a default reference clip (JFK sample) that is hardcoded or downloaded on the fly, but the documentation (`docs/SampleStore.md`) describes a structured sample store (`dictation`, `noise`, `voice`). Connecting benchmark runners directly to the sample store would yield more representative multi-clip accuracy metrics.
   *Smallest step*: Add an option to `voxi bench` to point it at `$XDG_DATA_HOME/voxi/samples/dictation` to run inference over user-specific challenging audio clips, not just the generic JFK clip.
   *Cite*: `docs/SampleStore.md`:27
