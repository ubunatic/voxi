package chunks

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/listing"
)

// NewCommand creates the `voxi chunks` command hierarchy.
func NewCommand(d deps.Dependencies, buf *Buffer) *cobra.Command {
	if buf == nil {
		buf = DefaultBuffer()
	}

	cmd := &cobra.Command{
		Use:   "chunks",
		Short: "Inspect, play, and debug recent recorded audio chunks and transcription metadata",
	}

	var reverse bool
	var listFormat string
	var colorMode string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent recorded chunks and their transcription outcomes",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !listing.IsValidColorMode(colorMode) {
				return fmt.Errorf("invalid --color value %q: must be one of %s", colorMode, strings.Join(listing.ValidColorModes, ", "))
			}
			chunks, err := buf.List(reverse)
			if err != nil {
				return err
			}
			if len(chunks) == 0 {
				fmt.Fprintln(d.Stdout, "No chunks recorded in ring buffer yet.")
				return nil
			}

			if listFormat == "json" {
				enc := json.NewEncoder(d.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(chunks)
			}

			var noColorEnv string
			if d.Getenv != nil {
				noColorEnv = d.Getenv("NO_COLOR")
			}
			useColor := listing.ShouldUseColor(colorMode, noColorEnv, listing.StdoutIsTerminal(d.Stdout))

			cols := []listing.Column{
				{Header: "INDEX", Width: 6},
				{Header: "TIMESTAMP", Width: 19},
				{Header: "AUDIO", Width: 6},
				{Header: "RTF", Width: 5},
				{Header: "RMS", Width: 5, Right: true},
				{Header: "LEVEL", Width: 12},
				{Header: "STATUS", Width: 10},
				{Header: "TRANSCRIPT", Width: 0},
			}
			rows := make([][]string, 0, len(chunks))
			for _, c := range chunks {
				status := "accepted"
				if !c.Accepted {
					if c.RejectionReason != "" {
						status = "rej:" + c.RejectionReason
					} else {
						status = "rejected"
					}
				}
				text := c.CleanedTranscript
				if text == "" && c.RawTranscript != "" {
					text = "[" + c.RawTranscript + "]"
				}
				if len(text) > 40 {
					text = text[:37] + "..."
				}
				ts := c.Timestamp.Local().Format("2006-01-02 15:04:05")
				// Bracket the sparkline so it reads as a bounded meter rather
				// than a stray glyph string floating in whitespace, and so a
				// missing/empty sparkline (chunks recorded before this field
				// existed) still shows a visible "[]" rather than nothing.
				// FormatSparklineCell pads to the column's fixed width
				// *before* colorizing -- see listing.ColorizeSparkline's doc
				// comment for why the order matters.
				level := listing.FormatSparklineCell(c.VolumeSparkline, 12, useColor)
				rows = append(rows, []string{
					fmt.Sprintf("#%d", c.Index),
					ts,
					fmt.Sprintf("%.1fs", c.AudioDurationSecs),
					fmt.Sprintf("%.2f", c.RTF),
					fmt.Sprintf("%d", c.MeanRMS),
					level,
					status,
					text,
				})
			}
			listing.WriteTable(d.Stdout, cols, rows)
			return nil
		},
	}
	listCmd.Flags().BoolVarP(&reverse, "reverse", "r", false, "list newest chunks first")
	listCmd.Flags().StringVar(&listFormat, "format", "text", "output format (text or json)")
	listCmd.Flags().StringVar(&colorMode, "color", listing.ColorAuto, "colorize the LEVEL sparkline by loudness: auto, always, or never")

	var showFormat string
	showCmd := &cobra.Command{
		Use:   "show [INDEX|last]",
		Short: "Show detailed diagnostics for a chunk (defaults to last)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			selector := "last"
			if len(args) > 0 {
				selector = args[0]
			}
			chunk, err := buf.Get(selector)
			if err != nil {
				return err
			}

			if showFormat == "text" {
				FormatChunkDetails(d.Stdout, chunk)
				return nil
			}
			enc := json.NewEncoder(d.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(chunk)
		},
	}
	showCmd.Flags().StringVar(&showFormat, "format", "json", "output format (json or text)")

	playCmd := &cobra.Command{
		Use:   "play [INDEX|last]",
		Short: "Play audio of a recorded chunk (defaults to last)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selector := "last"
			if len(args) > 0 {
				selector = args[0]
			}
			return buf.Play(cmd.Context(), d, selector)
		},
	}

	cmd.AddCommand(listCmd, showCmd, playCmd)
	return cmd
}

// FormatChunkDetails outputs human-readable chunk diagnostic fields.
func FormatChunkDetails(w io.Writer, c Chunk) {
	fmt.Fprintf(w, "Index:                  %d\n", c.Index)
	if c.SessionID != "" {
		fmt.Fprintf(w, "Session ID:             %s\n", c.SessionID)
		fmt.Fprintf(w, "Chunk ID:               %s\n", c.ChunkID)
	}
	fmt.Fprintf(w, "Timestamp:              %s\n", c.Timestamp.Local().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "Audio Duration:         %.2fs\n", c.AudioDurationSecs)
	fmt.Fprintf(w, "PCM Bytes:              %d\n", c.PCMBytes)
	fmt.Fprintf(w, "Mean / Peak RMS:        %d / %d\n", c.MeanRMS, c.PeakRMS)
	fmt.Fprintf(w, "Voiced Ratio:           %.3f\n", c.VoicedRatio)
	fmt.Fprintf(w, "Probable Silence:       %t\n", c.ProbableSilence)
	fmt.Fprintf(w, "Transcribe Duration:    %.2fs\n", c.TranscribeDurationSec)
	fmt.Fprintf(w, "Transcript Word Count:  %d\n", c.TranscriptWordCount)
	fmt.Fprintf(w, "RTF:                    %.2f\n", c.RTF)
	fmt.Fprintf(w, "Accepted:               %t\n", c.Accepted)
	if c.RejectionReason != "" {
		fmt.Fprintf(w, "Rejection Reason:       %s\n", c.RejectionReason)
	}
	fmt.Fprintf(w, "Raw Transcript:         %s\n", c.RawTranscript)
	fmt.Fprintf(w, "Cleaned Transcript:     %s\n", c.CleanedTranscript)
	fmt.Fprintf(w, "WAV File:               %s\n", c.WAVFile)
}
