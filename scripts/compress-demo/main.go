// Command compress-demo compresses a screencast recording into a small
// website demo asset: scales it down, caps at 30fps, and re-encodes with a
// high CRF -- screen recordings compress extremely well since most frames
// are near-static. Calibrated against website/voxi-demo.mp4 (2026-09-08):
// 31.7 MB / 1408x841 / 60fps -> 448 KB / 960px / 30fps, text still fully
// legible.
//
// START and END take a signed number of seconds: positive freezes the
// first/last frame for that long instead of cutting off abruptly; negative
// crops that much off the start/end instead.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func usage() {
	fmt.Fprint(os.Stderr, `Usage: compress-demo INPUT.mp4 [OUTPUT.mp4] [WIDTH] [START] [END]

  INPUT.mp4   source screencast (e.g. from GNOME's screen recorder)
  OUTPUT.mp4  default: INPUT with "-compressed" before the extension
  WIDTH       output width in pixels, height auto-scaled (default: 960)
  START       seconds at the start: positive freezes the first frame that
              long before playback begins; negative crops that much off
              the start (default: 0)
  END         seconds at the end: positive freezes the last frame that
              long instead of cutting off abruptly; negative crops that
              much off the end (default: 0)
`)
	os.Exit(1)
}

func main() {
	args := os.Args[1:]
	if len(args) < 1 {
		usage()
	}

	input := args[0]
	if _, err := os.Stat(input); err != nil {
		fatalf("no such file: %s", input)
	}

	output := input[:len(input)-len(filepath.Ext(input))] + "-compressed.mp4"
	if len(args) >= 2 {
		output = args[1]
	}
	width := "960"
	if len(args) >= 3 {
		width = args[2]
	}
	start := parseSeconds("START", args, 3)
	end := parseSeconds("END", args, 4)

	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			fatalf("%s not found on PATH", tool)
		}
	}

	hasAudio := probeHasAudio(input)
	totalDuration := probeDuration(input)

	startTrim, startPad := splitSigned(start)
	endTrim, endPad := splitSigned(end)

	encodeDuration := totalDuration - startTrim - endTrim
	if encodeDuration < 0 {
		encodeDuration = 0
	}

	videoFilter := fmt.Sprintf("scale=%s:-2,fps=30", width)
	var tpadParts []string
	if startPad > 0 {
		tpadParts = append(tpadParts, fmt.Sprintf("start_mode=clone:start_duration=%s", fmtSecs(startPad)))
	}
	if endPad > 0 {
		tpadParts = append(tpadParts, fmt.Sprintf("stop_mode=clone:stop_duration=%s", fmtSecs(endPad)))
	}
	if len(tpadParts) > 0 {
		videoFilter += ",tpad=" + strings.Join(tpadParts, ":")
	}

	ffmpegArgs := []string{"-y"}
	if startTrim > 0 {
		ffmpegArgs = append(ffmpegArgs, "-ss", fmtSecs(startTrim))
	}
	if startTrim > 0 || endTrim > 0 {
		// -t here is an INPUT option (it precedes -i), limiting how much of
		// the source is read to just the trimmed segment. Placed as an
		// output option instead, it would cap the final encoded duration
		// *after* filters -- clipping off any tpad padding added below.
		ffmpegArgs = append(ffmpegArgs, "-t", fmtSecs(encodeDuration))
	}
	ffmpegArgs = append(ffmpegArgs, "-i", input)
	ffmpegArgs = append(ffmpegArgs, "-vf", videoFilter,
		"-c:v", "libx264", "-crf", "28", "-preset", "veryslow", "-pix_fmt", "yuv420p",
		"-movflags", "+faststart")

	// Screen recordings are often silent (no mic captured); only pay for an
	// audio stream in the output when the input actually has one. adelay
	// shifts the audio to match a start freeze; apad extends it to match an
	// end freeze -- otherwise the frozen frame would play back silently past
	// where a shorter audio track ends.
	if hasAudio {
		var af []string
		if startPad > 0 {
			ms := int(startPad * 1000)
			af = append(af, fmt.Sprintf("adelay=%d|%d", ms, ms))
		}
		if endPad > 0 {
			af = append(af, fmt.Sprintf("apad=pad_dur=%s", fmtSecs(endPad)))
		}
		ffmpegArgs = append(ffmpegArgs, "-c:a", "aac", "-b:a", "96k")
		if len(af) > 0 {
			ffmpegArgs = append(ffmpegArgs, "-af", strings.Join(af, ","))
		}
	} else {
		ffmpegArgs = append(ffmpegArgs, "-an")
	}
	ffmpegArgs = append(ffmpegArgs, output, "-loglevel", "error")

	cmd := exec.Command("ffmpeg", ffmpegArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("ffmpeg failed: %v", err)
	}

	before := fileSize(input)
	after := fileSize(output)
	ratio := float64(before) / float64(after)

	fmt.Printf("Compressed %s -> %s\n", input, output)
	fmt.Printf("  before: %d bytes\n", before)
	fmt.Printf("  after:  %d bytes\n", after)
	fmt.Printf("  ratio:  %.1fx smaller\n", ratio)
}

// splitSigned separates a signed seconds value into a non-negative trim
// (how much to crop) and a non-negative pad (how much to freeze-extend).
func splitSigned(v float64) (trim, pad float64) {
	if v < 0 {
		return -v, 0
	}
	return 0, v
}

func fmtSecs(v float64) string {
	return strconv.FormatFloat(v, 'f', 3, 64)
}

func parseSeconds(name string, args []string, index int) float64 {
	if len(args) <= index {
		return 0
	}
	v, err := strconv.ParseFloat(args[index], 64)
	if err != nil {
		fatalf("%s: invalid seconds value %q", name, args[index])
	}
	return v
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

type ffprobeStreams struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
	} `json:"streams"`
}

func probeHasAudio(input string) bool {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "a",
		"-show_entries", "stream=codec_type", "-of", "json", input).Output()
	if err != nil {
		fatalf("ffprobe (audio detect) failed: %v", err)
	}
	var parsed ffprobeStreams
	if err := json.Unmarshal(out, &parsed); err != nil {
		fatalf("ffprobe (audio detect): parse output: %v", err)
	}
	return len(parsed.Streams) > 0
}

type ffprobeFormat struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func probeDuration(input string) float64 {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "json", input).Output()
	if err != nil {
		fatalf("ffprobe (duration) failed: %v", err)
	}
	var parsed ffprobeFormat
	if err := json.Unmarshal(out, &parsed); err != nil {
		fatalf("ffprobe (duration): parse output: %v", err)
	}
	d, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil {
		fatalf("ffprobe (duration): invalid duration %q", parsed.Format.Duration)
	}
	return d
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	os.Exit(1)
}
