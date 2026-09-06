#!/usr/bin/env python3
"""Canary runner for NVIDIA Nemotron 3.5 ASR streaming (0.6B) via the
official sherpa-onnx int8 export (issue 066).

Not production code -- throwaway canary harness, mirrors the
paired-comparison shape scripts/speech_context_bench/main.go already
established for whisper.cpp/voxtype.

Usage:
  python3 nemotron_run.py <model_dir> <wav_file> [wav_file ...]

Prints one JSON object per WAV to stdout: {"file":..., "transcript":...,
"latency_ms":..., "audio_s":..., "rtf":...}
"""
import json
import sys
import time
import wave

import sherpa_onnx


def load_recognizer(model_dir: str) -> sherpa_onnx.OnlineRecognizer:
    return sherpa_onnx.OnlineRecognizer.from_transducer(
        tokens=f"{model_dir}/tokens.txt",
        encoder=f"{model_dir}/encoder.int8.onnx",
        decoder=f"{model_dir}/decoder.int8.onnx",
        joiner=f"{model_dir}/joiner.int8.onnx",
        num_threads=6,
        sample_rate=16000,
        feature_dim=80,
        decoding_method="greedy_search",
    )


def run_one(recognizer: sherpa_onnx.OnlineRecognizer, wav_path: str) -> dict:
    with wave.open(wav_path, "rb") as w:
        assert w.getframerate() == 16000, f"{wav_path}: expected 16kHz, got {w.getframerate()}"
        assert w.getsampwidth() == 2, f"{wav_path}: expected 16-bit PCM"
        n = w.getnframes()
        raw = w.readframes(n)
    import array
    samples = array.array("h", raw)
    floats = [s / 32768.0 for s in samples]
    audio_s = len(floats) / 16000.0

    stream = recognizer.create_stream()
    started = time.time()
    # feed in 100ms chunks to emulate streaming ingestion
    chunk = 1600
    for i in range(0, len(floats), chunk):
        stream.accept_waveform(16000, floats[i:i + chunk])
        while recognizer.is_ready(stream):
            recognizer.decode_stream(stream)
    stream.input_finished()
    while recognizer.is_ready(stream):
        recognizer.decode_stream(stream)
    text = recognizer.get_result(stream)
    elapsed = time.time() - started
    return {
        "file": wav_path,
        "transcript": text,
        "latency_ms": int(elapsed * 1000),
        "audio_s": round(audio_s, 3),
        "rtf": round(elapsed / audio_s, 4) if audio_s > 0 else None,
    }


def main() -> None:
    if len(sys.argv) < 3:
        print(__doc__, file=sys.stderr)
        sys.exit(1)
    model_dir = sys.argv[1]
    recognizer = load_recognizer(model_dir)
    for wav_path in sys.argv[2:]:
        print(json.dumps(run_one(recognizer, wav_path)))


if __name__ == "__main__":
    main()
