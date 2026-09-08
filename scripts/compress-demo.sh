#!/usr/bin/env bash
set -euo pipefail

# Compress a screencast recording into a small website demo asset: scales
# down, caps at 30fps, and re-encodes with a high CRF -- screen recordings
# compress extremely well since most frames are near-static. Calibrated
# against website/voxi-demo.mp4 (2026-09-08): 31.7 MB / 1408x841 / 60fps
# -> 448 KB / 960px / 30fps, text still fully legible.

usage() {
   cat <<'EOF' >&2
Usage: compress-demo.sh INPUT.mp4 [OUTPUT.mp4] [WIDTH]

  INPUT.mp4   source screencast (e.g. from GNOME's screen recorder)
  OUTPUT.mp4  default: INPUT with "-compressed" before the extension
  WIDTH       output width in pixels, height auto-scaled (default: 960)
EOF
   exit 1
}

test "$#" -ge 1 || usage
input="${1:?Usage: compress-demo.sh INPUT.mp4 [OUTPUT.mp4] [WIDTH]}"
width="${3:-960}"

if ! test -f "$input"
then printf 'error: no such file: %s\n' "$input" >&2
     exit 1
fi

if ! command -v ffmpeg >/dev/null
then printf 'error: ffmpeg not found on PATH\n' >&2
     exit 1
fi

default_output="${input%.*}-compressed.mp4"
output="${2:-$default_output}"

# Screen recordings are often silent (no mic captured); only pay for an
# audio stream in the output when the input actually has one.
has_audio=$(ffprobe -v error -select_streams a -show_entries stream=codec_type \
   -of csv=p=0 "$input" 2>/dev/null | head -n1)

audio_args=(-an)
if test -n "$has_audio"
then audio_args=(-c:a aac -b:a 96k)
fi

ffmpeg -y -i "$input" \
   -vf "scale=${width}:-2,fps=30" \
   -c:v libx264 -crf 28 -preset veryslow -pix_fmt yuv420p \
   -movflags +faststart \
   "${audio_args[@]}" \
   "$output" \
   -loglevel error

before=$(stat -c%s "$input")
after=$(stat -c%s "$output")
ratio=$(awk -v b="$before" -v a="$after" 'BEGIN { printf "%.1f", b / a }')

printf 'Compressed %s -> %s\n' "$input" "$output"
printf '  before: %s bytes\n' "$before"
printf '  after:  %s bytes\n' "$after"
printf '  ratio:  %sx smaller\n' "$ratio"
