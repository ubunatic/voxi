#!/usr/bin/env bash
set -euo pipefail

cache_dir="$HOME/.cache/voxi"
output_dir="$HOME/.local/share/voxi/voice-demo"
sample_dir="$HOME/.config/voxi/samples"
sherpa_version="1.13.8"
model_version="2026-01-26"
sherpa_archive="sherpa-onnx-v${sherpa_version}-linux-x64-static.tar.bz2"
model_archive="sherpa-onnx-pocket-tts-int8-${model_version}.tar.bz2"
sherpa_sha256="d265986f7b990026e45bca573415575140106a2c663014924037d8f2c2e297cc"
model_sha256="2f3b88823cbbb9bf0b2477ec8ae7b3fec417b3a87b6bb5f256dba66f2ad967cb"
sherpa_root="$cache_dir/sherpa-onnx-v${sherpa_version}-linux-x64-static"
model_root="$cache_dir/sherpa-onnx-pocket-tts-int8-${model_version}"
binary="$sherpa_root/bin/sherpa-onnx-offline-tts"
reference_id="llama-lo-route"
reference_wav="$sample_dir/$reference_id.wav"

fail() {
   printf 'ERROR: %s\n' "$*" >&2
   exit 1
}

verify_sha256() {
   local expected="$1"
   local path="$2"
   local actual
   actual=$(sha256sum "$path" | awk '{print $1}')
   if test "$actual" != "$expected"
   then fail "SHA-256 mismatch for $path: expected $expected, got $actual"
   fi
}

download_pinned() {
   local filename="$1"
   local url="$2"
   local expected="$3"
   local path="$cache_dir/$filename"
   if ! test -f "$path"
   then curl -fL --retry 3 --connect-timeout 15 --max-time 600 -o "$path" "$url"
   fi
   verify_sha256 "$expected" "$path"
}

mkdir -p "$cache_dir" "$output_dir"
if ! test -f "$sample_dir/voice-training.txt"
then fail "missing sample allowlist: $sample_dir/voice-training.txt"
fi
if ! grep -Fxq "$reference_id" "$sample_dir/voice-training.txt"
then fail "reference ID is not in voice-training.txt: $reference_id"
fi
if ! test -f "$reference_wav"
then fail "missing allowlisted reference WAV: $reference_wav"
fi

download_pinned "$sherpa_archive" \
   "https://github.com/k2-fsa/sherpa-onnx/releases/download/v${sherpa_version}/${sherpa_archive}" \
   "$sherpa_sha256"
download_pinned "$model_archive" \
   "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/${model_archive}" \
   "$model_sha256"

if ! test -x "$binary"
then tar -xjf "$cache_dir/$sherpa_archive" -C "$cache_dir"
fi
if ! test -f "$model_root/lm_flow.int8.onnx"
then tar -xjf "$cache_dir/$model_archive" -C "$cache_dir"
fi
if ! test -x "$binary"
then fail "sherpa-onnx offline TTS binary is missing after extraction"
fi

synthesize() {
   local number="$1"
   local sentence="$2"
   local output="$output_dir/demo-${number}.wav"
   local started
   local finished
   local elapsed_ms
   started=$(date +%s%N)
   "$binary" \
      --pocket-lm-flow="$model_root/lm_flow.int8.onnx" \
      --pocket-lm-main="$model_root/lm_main.int8.onnx" \
      --pocket-encoder="$model_root/encoder.onnx" \
      --pocket-decoder="$model_root/decoder.int8.onnx" \
      --pocket-text-conditioner="$model_root/text_conditioner.onnx" \
      --pocket-vocab-json="$model_root/vocab.json" \
      --pocket-token-scores-json="$model_root/token_scores.json" \
      --reference-audio="$reference_wav" \
      --num-threads=6 \
      --output-filename="$output" \
      "$sentence"
   finished=$(date +%s%N)
   elapsed_ms=$(((finished - started) / 1000000))
   printf 'demo-%s.wav: %s ms from process start to complete WAV (offline first-audio bound)\n' \
      "$number" "$elapsed_ms"
}

synthesize 1 "Good morning. I hope your day is off to a calm and pleasant start."
synthesize 2 "Please send me the latest notes when you have a moment."
synthesize 3 "I will be back shortly, so feel free to continue without me."

printf 'Created demo WAVs in %s using reference %s.wav\n' "$output_dir" "$reference_id"
