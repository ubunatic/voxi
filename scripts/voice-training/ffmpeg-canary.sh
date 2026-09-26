#!/usr/bin/env bash
set -euo pipefail

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

ffmpeg -v error -f lavfi -i sine=frequency=440:duration=0.1 -ar 44100 -ac 2 "$tmpdir/source.wav"
ffmpeg -v error -nostdin -i "$tmpdir/source.wav" -ar 22050 -ac 1 -c:a pcm_s16le "$tmpdir/normalized.wav"
ffprobe -v error -show_entries stream=sample_rate,channels,sample_fmt -of default=noprint_wrappers=1 "$tmpdir/normalized.wav"
