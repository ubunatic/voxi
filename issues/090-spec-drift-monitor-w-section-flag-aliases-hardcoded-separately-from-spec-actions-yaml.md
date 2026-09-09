# 090 — Spec drift: `monitor -w` section-flag aliases hardcoded separately from spec/actions.yaml

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Cleanup (spec-shadowing, no user-facing breakage observed yet)
**Category**: Spec System / Code Quality
**Related**: [internal/monitor/monitor.go](../internal/monitor/monitor.go) (`ParseSections`), [spec/actions.yaml](../spec/actions.yaml), [spec/actions.go](../spec/actions.go), [docs/Spec.md](../docs/Spec.md)

---

## 1. Problem

`docs/Spec.md` names `spec/actions.yaml` the single source of truth for the
monitor TUI's section identifiers, and `spec/actions.go`'s doc comment says so
explicitly: "Application code must not hardcode key literals that duplicate or
shadow this list." `internal/monitor/monitor.go`'s runtime hotkey dispatch
(`loadedActions().KeyDispatch()`, used at monitor.go:160) correctly reads from
the spec.

But `ParseSections` (monitor.go:88-111), which parses the **startup** `-w`
flag value (e.g. `voxi monitor -w hardware,transcript`), maintains its own,
independently-hand-written alias table instead of resolving through
`spec.LoadActions()`:

```go
switch p {
case "s", "speed", "status", "voice", "v", "1":
        sec.Speed = true
case "h", "hardware", "cpu", "gpu", "hw", "c", "g", "2":
        sec.Hardware = true
case "t", "transcript", "sentences", "feed", "3":
        sec.Transcript = true
case "d", "daemons", "procs", "health", "p", "4":
        sec.Daemons = true
}
```

Compare against `spec/actions.yaml`'s actual keys for the same four sections:

| Section | spec/actions.yaml keys | ParseSections aliases |
|---|---|---|
| speed | `s S 1 v V` | `s speed status voice v 1` |
| hardware | `h H 2 c C g G` | `h hardware cpu gpu hw c g 2` |
| transcript | `t T 3` | `t transcript sentences feed 3` |
| daemons | `d D 4 p P` | `d daemons procs health p 4` |

The two lists overlap but are not the same vocabulary: `ParseSections`
recognizes long words (`"status"`, `"voice"`, `"sentences"`, `"feed"`,
`"procs"`, `"health"`, `"hw"`) that don't exist anywhere in the spec, while
the spec's canonical short titles (`Action.Short`: `"speed"`, `"hardware"`,
`"transcript"`, `"daemons"`) happen to be *among* the hardcoded aliases only
by accident of hand-copying, not by being read from `a.Short`.

## 2. Impact

- Adding, renaming, or removing a section in `spec/actions.yaml` (e.g. to
  match a future TUI section) silently does **not** change what `-w` accepts
  — the two vocabularies can drift further apart with no compiler or test
  signal, which is exactly the "silent drift" anti-pattern `docs/Spec.md` §1
  calls out.
- `spec/actions_test.go` presumably tests `LoadActions`/`KeyDispatch` in
  isolation; it cannot catch this because `ParseSections` never calls into
  `spec` at all.

## 3. Suggested Fix

Have `ParseSections` resolve against `spec.LoadActions()`'s `Action.Short`
field (and single-character keys already validated by `parseActionSpec`)
instead of a hand-maintained `switch`. Add a test asserting every alias
`ParseSections` accepts is derivable from the loaded spec, so a future
spec change is forced to touch (or intentionally diverge from) this parser.
