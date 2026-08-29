# 031: Claude Code and agent CLI voice pipeline research

**Status**: Proposed / Research  
**Category**: Voice Transcription & Agent CLI Research  

---

## 1. Objective

Research reliable voice capture / voice command pipelines for CLI coding
agents, especially Claude Code, and document what `voxi` can learn from them.

Treat transcript variants like "boxy", "waxy", "Voxy", or "VoxType" as `voxi`
unless context clearly says otherwise.

## 2. Research Questions

- What does Claude Code use for voice capture and transcription, if that is
  discoverable from local installation artifacts, public source/code bundles,
  remote copies, reverse-engineering notes, or official documentation?
- What is the likely end-to-end Claude Code voice pipeline: trigger/hotkey,
  audio capture, local vs remote processing, transcription provider/model,
  correction or prompt-injection path into the CLI, and privacy/network
  implications?
- Which facts are confirmed by source/docs and which parts remain informed
  inference?
- Are there other open-source agent CLI tools with voice command workflows that
  users actually appear to use and find helpful?
- What can `voxi` adopt or test to reduce current ASR friction, especially
  hallucinated proper nouns, odd YouTube/person names, and command-mode
  misrecognitions?

## 3. Research Scope

- Inspect local Claude Code installation artifacts available on this machine.
- Search the web for Claude Code voice capture, transcription internals, and
  community reverse-engineering notes.
- Prefer primary sources: official docs, public repositories, installed bundles,
  package metadata, and source files.
- Use secondary/community sources only to guide investigation or to identify
  artifacts worth verifying.
- Look for other open-source CLI/dev-agent voice systems with concrete usage
  evidence, not generic "voice assistant" lists.

## 4. Deliverable

Update this ticket with:

- confirmed Claude Code pipeline findings;
- source links and local file references;
- explicit uncertainty/inference notes;
- comparison to other plausible CLI/agent voice workflows;
- concrete `voxi` follow-up recommendations;
- any high-value experiments to improve transcription quality and command
  usefulness.

## 5. Non-goals

- Do not implement code in this research task.
- Do not change `voxi` transcription behavior yet.
- Do not claim Claude Code internals are known unless backed by source,
  installed artifacts, or official documentation.

