# miclevel

Live microphone level meter and scrolling Braille sparkline, rendered in a
`loom` TUI pane and fed by voxi's own `audiolevel` package — a real,
non-simulated demonstration that `audiolevel`'s exported API
(`StartManager`, `Meter.EnableSparkline`) is consumable by an external Go
program, and `loom`'s first live external-source integration (its own
`examples/monitor` deliberately stays on simulated data).

From the repository root:

```sh
go run ./examples/miclevel
```

Speak into your microphone: the `Level` row's bar and percentage, and the
`Wave` row's scrolling sparkline, should respond within a couple hundred
milliseconds and decay back down when you stop. Press `q` or Ctrl-C to quit
(restores the terminal cleanly).

If neither `parec` nor `pw-record` is on `PATH`, the pane still opens but
shows a `(no mic backend found: need parec or pw-record)` footer with the
bar pinned at `--%` and the wave row reading `(no backend)` — no panic, no
crash.

## Files

- [main.go](main.go): thin Cobra entrypoint, embeds `spec/miclevel.yaml`.
- [watch.go](watch.go): picks a capture backend, starts
  `audiolevel.StartManager`, and drives `loom.Pane.RunWatch` — each collect
  tick calls `Manager.Tick` (ballistics-eased motion between raw audio
  chunks, see `audiolevel.Meter.Tick`) and writes the formatted bar/percent
  and sparkline into the declared box's rows via `Box.SetRowsValues`.
- [spec/miclevel.yaml](spec/miclevel.yaml): the `loom` frame/box layout —
  edit it to change titles, widths, or column layout without touching Go.

## Local development against an uncommitted loom checkout

This module depends on a tagged `codeberg.org/ubunatic/loom` release by
default. To build against a sibling `../loom` working copy instead (e.g.
testing an in-flight loom change before it's tagged), copy the tracked
example workspace once:

```sh
cp go.work.example go.work
```

`go.work` is untracked (`.gitignore`) and, once present, transparently
overrides the pinned dependency for every `go build`/`go test`/`go run` in
this repo — remove it to go back to the pinned release. See
`docs/Go.md` "Workspace Isolation" for the general convention.

## Privacy

Like every other mic-level consumer in this repo, this example opens a real
capture stream tagged with `audiolevel.SuppressApplicationID` so GNOME's and
KDE's mic-in-use indicators exempt it (same exemption `voxi monitor` already
relies on) — see `audiolevel`'s package doc comment. No audio is ever
written anywhere; only the level/sparkline it derives is displayed.
