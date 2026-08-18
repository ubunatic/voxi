---
title: Bench Baseline
weight: 66
---

# Bench Baseline

Reference `voxi bench` numbers for an average dev machine, to reason about
model/backend choices without re-running the benchmark. Re-baseline this
file whenever `spec/models.yaml` gains a model or the reference hardware
changes materially.

## Reference machine

- **Model**: Lenovo ThinkPad T14 Gen2 AMD
- **CPU**: AMD Ryzen 5 PRO 5650U (6c/12t)
- **GPU**: AMD Radeon Graphics (Cezanne/RADV RENOIR, Vulkan 1.4, `/dev/dri/renderD128`)
- **RAM**: 24GB
- **OS**: Fedora, kernel 7.1.7 (Linux/Wayland)
- **Date**: 2026-08-18

## Command

```sh
voxi bench
```

Uses the default reference clip (whisper.cpp's JFK sample, downloaded and
checksum-verified into `~/.cache/voxi/bench/`, never committed — see
`internal/bench`), every model in `spec/models.yaml`, and both `gpu` and
`cpu` backends since a GPU render node is present.

## Results

| Model | Backend | RTF | Speedup | Detected |
| :--- | :--- | ---: | ---: | :--- |
| base.en | gpu | 0.06 | 16.02x | gpu:Vulkan0 |
| small.en | gpu | 0.18 | 5.44x | gpu:Vulkan0 |
| large-v3-turbo | gpu | 0.59 | 1.69x | gpu:Vulkan0 |
| base.en | cpu | 0.17 | 5.81x | cpu |
| small.en | cpu | 0.53 | 1.87x | cpu |
| large-v3-turbo | cpu | 2.54 | 0.39x | cpu |

## Reading it

- **GPU speedup grows with model size**: ~2.8x for `base.en`/`small.en`,
  ~4.3x for `large-v3-turbo`. GPU acceleration matters most for the models
  it's least optional for.
- **`large-v3-turbo` on CPU is sub-realtime** (RTF 2.54, 0.39x speedup) — it
  falls behind live speech and accumulates latency. Do not select it for
  eager/continuous mode without confirmed GPU availability
  (`internal/monitor` GPU detection, or `voxtype info variants`).
- **`base.en` on GPU has the largest realtime margin** (16x), making it the
  safe fallback when GPU is present but headroom is uncertain (background
  load, thermal throttling, concurrent inference).
- All three models are realtime-safe on GPU; only `base.en` and `small.en`
  are comfortably realtime-safe on CPU alone.

## Reproducing / updating this baseline

Re-run `voxi bench --json out.json` on representative hardware, update the
table above, and note the machine + date. Keep only one baseline per
representative machine class here; log one-off or exploratory runs
elsewhere (e.g. `docs/studies/`) instead of accumulating stale tables.
