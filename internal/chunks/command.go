package chunks

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
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
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent recorded chunks and their transcription outcomes",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
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

			fmt.Fprintf(d.Stdout, "%-6s  %-19s  %-7s  %-5s  %-10s  %s\n", "INDEX", "TIMESTAMP", "AUDIO", "RTF", "STATUS", "TRANSCRIPT")
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
				fmt.Fprintf(d.Stdout, "#%-5d  %-19s  %5.1fs  %5.2f  %-10s  %s\n",
					c.Index, ts, c.AudioDurationSecs, c.RTF, status, text)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVarP(&reverse, "reverse", "r", false, "list newest chunks first")
	listCmd.Flags().StringVar(&listFormat, "format", "text", "output format (text or json)")

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
