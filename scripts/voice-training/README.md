# Local Piper training

This harness uses the Piper project's training module in a project-local uv
environment. It does not download a voice checkpoint; provide one explicitly.

```sh
uv sync
uv run python runner.py \
  --dataset ~/.local/share/voxi/voice-training/dataset \
  --name my-voice \
  --base /path/to/en_US-lessac-medium.ckpt \
  --espeak-voice en-us \
  --epochs 100
```

The run writes logs/checkpoints below `--output-dir` and exports
`<name>.onnx` plus `<name>.onnx.json` after training succeeds. Select an
accelerator explicitly with `--accelerator cpu|gpu`; `gpu` requires a working
PyTorch GPU backend. For available Piper options, use
`uv run python -m piper.train fit --help`.

Training is resource intensive. The prepared corpus and model checkpoints are
private user data; keep them out of source control.
