#!/usr/bin/env python3
"""CPU-only TTS engine canary; optional neural engines are enabled by env vars.

Run: python3 scripts/canary_tts/benchmark.py
Optional: VOXI_PIPER_MODEL=/path/voice.onnx [VOXI_PIPER_CONFIG=/path/voice.onnx.json]
          VOXI_KOKORO_MODEL=/path/kokoro-v1.0.onnx VOXI_KOKORO_VOICES=/path/voices-v1.0.bin
The script never downloads models and pins ONNX Runtime to CPUExecutionProvider.
"""

import os
import shutil
import subprocess
import tempfile
import time
import wave
from pathlib import Path

TEXT = "Hello. This is a short local speech synthesis benchmark."
ROOT = Path(__file__).resolve().parents[2]


def run(label, argv):
    if not shutil.which(argv[0]):
        print(f"{label}: unavailable ({argv[0]} not found)")
        return
    with tempfile.TemporaryDirectory(prefix="voxi-tts-canary-") as tmp:
        wav_path = Path(tmp) / "sample.wav"
        text_path = Path(tmp) / "input.txt"
        text_path.write_text(TEXT)
        command = [part.replace("{wav}", str(wav_path)).replace("{textfile}", str(text_path)) for part in argv]
        timed = shutil.which("/usr/bin/time")
        if timed:
            command = [timed, "-f", "VOXI_METRICS %U %S %M", *command]
        start = time.perf_counter()
        result = subprocess.run(command, input=TEXT, text=True, stdout=subprocess.DEVNULL,
                                stderr=subprocess.PIPE, check=False)
        elapsed = time.perf_counter() - start
        metric = ""
        stderr = result.stderr
        if "VOXI_METRICS " in stderr:
            stderr, metrics = stderr.rsplit("VOXI_METRICS ", 1)
            user_cpu, system_cpu, rss_kb = metrics.strip().split()[:3]
            metric = f" CPU={float(user_cpu) + float(system_cpu):.2f}s peak-RSS={int(rss_kb) / 1024:.1f}MiB"
        if result.returncode:
            print(f"{label}: failed ({stderr.strip()[-400:]})")
            return
        try:
            with wave.open(str(wav_path)) as audio:
                duration = audio.getnframes() / audio.getframerate()
        except (OSError, wave.Error) as exc:
            print(f"{label}: no readable WAV ({exc})")
            return
        print(f"{label}: latency={elapsed:.3f}s audio={duration:.2f}s RTF={elapsed / duration:.3f}{metric} WAV={wav_path.stat().st_size}B")


def main():
    if not shutil.which("/usr/bin/time"):
        print("Note: install GNU time to add per-process CPU and peak-RSS measurements.")
    print(f"CPU canary; host={ROOT}; utterance={TEXT!r}")
    run("Festival", ["text2wave", "-o", "{wav}"])
    run("espeak-ng", ["espeak-ng", "-w", "{wav}", "--stdin"])
    if model := os.getenv("VOXI_PIPER_MODEL"):
        config = os.getenv("VOXI_PIPER_CONFIG", model + ".json")
        python = os.getenv("VOXI_TTS_PYTHON", "python3")
        run("Piper", [python, "-m", "piper", "--model", model, "--config", config,
                      "--input_file", "{textfile}", "--output_file", "{wav}"])
    else:
        print("Piper: skipped (set VOXI_PIPER_MODEL to a downloaded voice ONNX file)")
    if os.getenv("VOXI_KOKORO_MODEL") and os.getenv("VOXI_KOKORO_VOICES"):
        python = os.getenv("VOXI_TTS_PYTHON", "python3")
        run("Kokoro", [python, str(Path(__file__).with_name("kokoro_once.py")), "{wav}"])
    else:
        print("Kokoro: skipped (set VOXI_KOKORO_MODEL and VOXI_KOKORO_VOICES)")


if __name__ == "__main__":
    main()
