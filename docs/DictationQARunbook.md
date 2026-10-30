# Dictation QA Runbook

Repeatable, paired validation of microphone capture, ASR, typing and offline-server behavior. The operator runs commands; the QA assistant speaks, watches the display and reports observations. Do not include private transcripts in shared logs or tickets without consent.

## Safety and baseline

- Start with a focused disposable text document and a terminal for `voxi monitor`. Record the selected microphone, model and service state so changes can be reversed.
- Do not stop the proxy or ASR service as part of the normal checks: other applications may use them. Fault injection is an optional, separately announced maintenance step; restore any services and settings to their original states even on failure.
- `voxi doctor --fix` may change audio routing or restart services; review its proposed commands. Do not use `--yes` blindly. A WARN is not automatically a fault.

## 1. Baseline

Operator runs:

```bash
voxi settings --dump
voxi mic
pactl get-default-source
systemctl --user status voxi-agent.service --no-pager -l
voxi doctor
```

Record the selected model, active daemon model, microphone, and doctor results. Doctor's ASR probe checks transcription of a test clip, not microphone-to-desktop dictation. Local engines may report unverified. If the model or microphone is not the intended one, select it and restart the agent before testing; note the previous setting.

## 2. Observe and dictate

1. Operator starts `voxi monitor` in a visible terminal. QA describes the model, backend and any warning before recording.
2. QA focuses a disposable text document, toggles recording with Super+X, speaks a short agreed sentence, and toggles again. QA reports what appeared and any delay; do not share the sentence unless comfortable.
3. Operator inspects `voxi chunks list` and `voxi chunks show`. If needed, operator runs `voxi chunks play`; QA says whether the recording contains clear speech, noise or silence.
4. Distinguish capture (playback), transcription (chunk result), and typing (text in the document). A healthy monitor alone is not an end-to-end pass.

## 3. Offline remote

If the remote server is already offline, repeat steps 1 and 2. Record whether doctor and actual dictation work and whether the backend used is demonstrable; a successful proxy probe does not identify which backend served it. Do not deliberately disconnect the remote host in a routine run.

## 4. Optional fault and repair validation

For issue 206 closure, schedule a maintenance window and record which proxy and local ASR services were active. Tell the user which other applications may be interrupted. Inject one controlled failure at a time, inspect `voxi doctor`, and confirm that it either offers a usable repair or reports an actionable failure without claiming success. Check proposed repairs and never rely on unavailable fallback model files. Run `voxi doctor --fix` interactively only when safe; verify its recheck and repeat step 2 after an actual repair. Restore every service and setting to its recorded prior state, including on interruption. If a safe injection or restoration cannot be guaranteed, stop and report the unverified case rather than closing the issue.

## 5. Results

Record pass/fail separately for: service and selected model; microphone playback; ASR transcript; desktop typing; remote-offline operation; and optional failure/repair. Note device, model, warnings, observed errors, changes and restoration. Never infer a repair pass from a service listening on a port alone.
