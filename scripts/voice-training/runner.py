#!/usr/bin/env python3
"""Run Piper fine-tuning and export a native ONNX voice."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from datetime import datetime
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dataset", type=Path, required=True)
    parser.add_argument("--name", required=True)
    parser.add_argument("--base", type=Path, required=True, help="Piper medium training checkpoint")
    parser.add_argument("--espeak-voice", required=True)
    parser.add_argument("--epochs", type=int, default=100)
    parser.add_argument("--batch-size", type=int, default=16)
    parser.add_argument("--accelerator", choices=("cpu", "gpu"), default="cpu")
    parser.add_argument("--output-dir", type=Path, default=Path("runs"))
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    dataset = args.dataset.expanduser().resolve()
    base = args.base.expanduser().resolve()
    output = args.output_dir.expanduser().resolve()
    metadata = dataset / "metadata.csv"
    wavs = dataset / "wavs"
    if not metadata.is_file() or not wavs.is_dir():
        raise SystemExit(f"dataset must contain metadata.csv and wavs/: {dataset}")
    if not base.is_file():
        raise SystemExit(f"base checkpoint does not exist: {base}")
    if args.epochs < 1 or args.batch_size < 1:
        raise SystemExit("epochs and batch size must be positive")
    if not args.name or any(c not in "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-" for c in args.name):
        raise SystemExit("name must contain only ASCII letters, digits, underscores, and hyphens")
    if shutil.which("espeak-ng") is None:
        raise SystemExit("espeak-ng is required by Piper for phonemization")
    output.mkdir(parents=True, exist_ok=True)
    runs = output / "runs"
    runs.mkdir(exist_ok=True)
    run_dir = runs / f"{args.name}-{datetime.now().strftime('%Y%m%d-%H%M%S-%f')}"
    run_dir.mkdir()
    cache = run_dir / "cache"
    cache.mkdir(exist_ok=True)
    config = run_dir / f"{args.name}.onnx.json"
    final_onnx = output / f"{args.name}.onnx"
    final_config = output / f"{args.name}.onnx.json"
    fit = [
        sys.executable, "-m", "piper.train", "fit",
        f"--data.voice_name={args.name}",
        f"--data.csv_path={metadata}",
        f"--data.audio_dir={wavs}",
        "--model.sample_rate=22050",
        f"--data.espeak_voice={args.espeak_voice}",
        f"--data.cache_dir={cache}",
        f"--data.config_path={config}",
        f"--data.batch_size={args.batch_size}",
        f"--trainer.max_epochs={args.epochs}",
        f"--trainer.accelerator={'gpu' if args.accelerator == 'gpu' else 'cpu'}",
        f"--ckpt_path={base}",
        f"--trainer.default_root_dir={run_dir}",
    ]
    subprocess.run(fit, check=True, cwd=run_dir)
    checkpoints = sorted(run_dir.glob("**/*.ckpt"), key=lambda path: path.stat().st_mtime)
    if not checkpoints:
        raise SystemExit(f"training completed without a checkpoint under {run_dir}")
    onnx = run_dir / f"{args.name}.onnx"
    subprocess.run([
        sys.executable, "-m", "piper.train.export_onnx",
        "--checkpoint", str(checkpoints[-1]),
        "--output-file", str(onnx),
    ], check=True, cwd=run_dir)
    if not onnx.is_file() or not config.is_file():
        raise SystemExit("Piper export did not produce both ONNX model and JSON config")
    try:
        config_data = json.loads(config.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as err:
        raise SystemExit(f"Piper export produced an invalid JSON config: {err}") from err
    if not isinstance(config_data, dict):
        raise SystemExit("Piper export config must contain a JSON object")
    publish_artifacts(onnx, config, final_onnx, final_config, output)
    print(f"exported {final_onnx} and {final_config}")
    return 0


def publish_artifacts(onnx: Path, config: Path, final_onnx: Path, final_config: Path, output: Path) -> None:
    """Stage both exports before replacing either prior destination artifact."""
    with tempfile.TemporaryDirectory(prefix=".voice-export-", dir=output) as temporary:
        stage = Path(temporary)
        staged_onnx = stage / final_onnx.name
        staged_config = stage / final_config.name
        backup_onnx = stage / "previous.onnx"
        backup_config = stage / "previous.onnx.json"
        shutil.copyfile(onnx, staged_onnx)
        shutil.copyfile(config, staged_config)

        had_onnx = had_config = False
        published_onnx = published_config = False
        try:
            if final_onnx.exists() or final_onnx.is_symlink():
                os.replace(final_onnx, backup_onnx)
                had_onnx = True
            if final_config.exists() or final_config.is_symlink():
                os.replace(final_config, backup_config)
                had_config = True
            os.replace(staged_config, final_config)
            published_config = True
            os.replace(staged_onnx, final_onnx)
            published_onnx = True
        except OSError:
            if published_onnx:
                final_onnx.unlink(missing_ok=True)
            if published_config:
                final_config.unlink(missing_ok=True)
            if had_onnx:
                os.replace(backup_onnx, final_onnx)
            if had_config:
                os.replace(backup_config, final_config)
            raise


if __name__ == "__main__":
    raise SystemExit(main())
