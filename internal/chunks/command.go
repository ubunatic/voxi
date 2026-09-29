package chunks

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
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
				{Header: "STATUS", Width: 12},
				{Header: "TRANSCRIPT / REASON", Width: 0},
			}
			rows := make([][]string, 0, len(chunks))
			for _, c := range chunks {
				status := FormatStatusBadges(c)
				var text string
				if c.Accepted {
					text = c.CleanedTranscript
					if text == "" && c.RawTranscript != "" {
						text = "[" + c.RawTranscript + "]"
					}
				} else {
					if c.RejectionReason != "" {
						text = "(" + c.RejectionReason + ")"
					} else {
						text = "(rejected)"
					}
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

	var deleteAll bool
	var deleteYes bool
	deleteCmd := &cobra.Command{
		Use:   "delete [INDEX|last]",
		Short: "Delete recorded chunk audio and transcript metadata",
		Long:  "Delete one recorded chunk by index or 'last', or use --all to delete every stored chunk.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if deleteAll && len(args) != 0 {
				return fmt.Errorf("delete accepts either INDEX|last or --all, not both; see --help for usage")
			}
			if !deleteAll && len(args) != 1 {
				return fmt.Errorf("delete requires INDEX|last or --all; see --help for usage")
			}
			if deleteAll {
				if !deleteYes && isInteractiveInput(d.Stdin) {
					chunks, err := buf.List(false)
					if err != nil {
						return err
					}
					promptOut := d.Stderr
					if promptOut == nil {
						promptOut = os.Stderr
					}
					fmt.Fprintf(promptOut, "Delete all %d recorded chunks? [y/N] ", len(chunks))
					answer, err := bufio.NewReader(d.Stdin).ReadString('\n')
					if err != nil && len(answer) == 0 {
						return fmt.Errorf("read delete confirmation: %w", err)
					}
					if answer = strings.TrimSpace(answer); !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
						fmt.Fprintln(promptOut, "Deletion cancelled.")
						return nil
					}
				}
				return buf.Delete("all")
			}
			return buf.Delete(args[0])
		},
	}
	deleteCmd.Flags().BoolVar(&deleteAll, "all", false, "delete every stored chunk")
	deleteCmd.Flags().BoolVarP(&deleteYes, "yes", "y", false, "skip the interactive confirmation for --all")

	cmd.AddCommand(listCmd, showCmd, playCmd, deleteCmd)
	return cmd
}

func isInteractiveInput(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// FormatStatusBadges formats the outcome (✓ or ✗) and effective pipeline stage badges for a chunk.
func FormatStatusBadges(c Chunk) string {
	var badges []string
	if c.Accepted {
		badges = append(badges, "✓")
	} else {
		badges = append(badges, "✗")
	}

	// Engine badge: only show if transcription actually ran / produced a transcript
	if c.TranscribeDurationSec > 0 || c.RawTranscript != "" {
		switch {
		case c.Engine == "whisper" || isWhisperModel(c.Model):
			badges = append(badges, "👂")
		case c.Engine == "openai-transcribe" && strings.Contains(c.Model, "gemini"):
			// openai-transcribe-gemini specifically (see issue 126): agy's
			// Gemini voice RPC behind whisper-server, not a whisper.cpp
			// model -- distinct from the generic openai-transcribe badge
			// below so it doesn't read as an unbranded/unknown remote call.
			badges = append(badges, "🔷")
		case c.Engine == "openai-transcribe":
			// Remote HTTP call (see issue 126), unlike the other two
			// engines which run a local binary -- distinct badge so it
			// doesn't read as the same local-fast-path as cohere-transcribe.
			badges = append(badges, "🌐")
		default:
			badges = append(badges, "⚡")
		}
	}

	// Deterministic replacements applied
	if len(c.AppliedReplacements) > 0 {
		badges = append(badges, "⇄")
	}

	// LLM post-processing cleaner modified text
	if c.LLMCleanup != nil && c.LLMCleanup.Modified {
		badges = append(badges, "🤖")
	}

	// Stop-word hallucination pattern matched or stripped
	if len(c.StopWordsMatched) > 0 || strings.HasPrefix(c.RejectionReason, "stop_word") {
		badges = append(badges, "✂")
	}

	// Transcript safety breaker tripped
	if isSafetyRejection(c.RejectionReason) {
		badges = append(badges, "🛡")
	}

	return strings.Join(badges, " ")
}

func isWhisperModel(model string) bool {
	if model == "" {
		return false
	}
	return strings.Contains(model, "whisper") || model == "base.en" || model == "small.en" || model == "large-v3-turbo"
}

func isSafetyRejection(reason string) bool {
	switch reason {
	case "output_too_long", "token_too_long", "pathological_repetition":
		return true
	default:
		return false
	}
}

// FormatChunkDetails outputs human-readable chunk diagnostic fields and pipeline trace.
func FormatChunkDetails(w io.Writer, c Chunk) {
	fmt.Fprintf(w, "Chunk #%d Diagnostics & Pipeline Summary:\n", c.Index)
	fmt.Fprintln(w, "==================================================================")
	fmt.Fprintf(w, "Timestamp:              %s (Audio: %.2fs, RTF: %.2f)\n", c.Timestamp.Local().Format("2006-01-02 15:04:05"), c.AudioDurationSecs, c.RTF)

	engineDesc := ""
	if c.Engine == "cohere-transcribe" {
		model := c.Model
		if model == "" {
			model = "cohere-transcribe-03-2026"
		}
		engineDesc = fmt.Sprintf("%s (via crispasr)", model)
	} else if c.Engine == "whisper" || isWhisperModel(c.Model) {
		model := c.Model
		if model == "" {
			model = "whisper"
		}
		engineDesc = fmt.Sprintf("%s (via voxtype)", model)
	} else if c.Model != "" {
		if c.Engine != "" {
			engineDesc = fmt.Sprintf("%s (%s)", c.Model, c.Engine)
		} else {
			engineDesc = c.Model
		}
	} else if c.Engine != "" {
		engineDesc = c.Engine
	} else {
		engineDesc = "unknown"
	}
	fmt.Fprintf(w, "ASR Engine:             %s\n", engineDesc)

	if c.Accepted {
		fmt.Fprintln(w, "Status:                 ACCEPTED")
	} else if c.RejectionReason != "" {
		fmt.Fprintf(w, "Status:                 REJECTED (%s)\n", c.RejectionReason)
	} else {
		fmt.Fprintln(w, "Status:                 REJECTED")
	}

	fmt.Fprintln(w, "\nPipeline Transformations:")
	fmt.Fprintln(w, "------------------------------------------------------------------")
	if c.RawTranscript != "" {
		fmt.Fprintf(w, "1. Raw ASR Output:      %q\n", c.RawTranscript)
	} else {
		fmt.Fprintln(w, "1. Raw ASR Output:      (none)")
	}

	if len(c.AppliedReplacements) > 0 {
		var repls []string
		for _, r := range c.AppliedReplacements {
			repls = append(repls, fmt.Sprintf("%q -> %q", r.From, r.To))
		}
		fmt.Fprintf(w, "2. Replacements:        %s\n", strings.Join(repls, ", "))
	} else {
		fmt.Fprintln(w, "2. Replacements:        (none)")
	}

	if c.LLMCleanup != nil && c.LLMCleanup.Enabled {
		model := c.LLMCleanup.Model
		if model == "" {
			model = "qwen3-4b-instruct-2507-q4"
		}
		fmt.Fprintf(w, "3. LLM Cleanup:         %s (via lmcoder)\n", model)
		fmt.Fprintf(w, "   LLM Output:          %q\n", c.LLMCleanup.Output)
	} else {
		fmt.Fprintln(w, "3. LLM Cleanup:         (disabled)")
	}

	if c.Accepted {
		fmt.Fprintf(w, "4. Final Committed:     %q\n", c.CleanedTranscript)
	} else if c.RejectionReason != "" {
		fmt.Fprintf(w, "4. Final Committed:     (rejected: %s)\n", c.RejectionReason)
	} else {
		fmt.Fprintln(w, "4. Final Committed:     (rejected)")
	}

	fmt.Fprintln(w, "\nInjection:")
	fmt.Fprintln(w, "------------------------------------------------------------------")
	if c.Accepted {
		fmt.Fprintln(w, "Destination:            Focused Window via dotool (type_delay_ms = 0ms)")
	} else {
		fmt.Fprintln(w, "Destination:            (none - rejected)")
	}

	if !c.TypingStartedAt.IsZero() && !c.TypingEndedAt.IsZero() {
		typingDur := c.TypingEndedAt.Sub(c.TypingStartedAt)
		typeMs := typingDur.Milliseconds()
		totalDurSec := c.TranscribeDurationSec + typingDur.Seconds()
		fmt.Fprintf(w, "Latency:                %.2fs transcribe + %dms typing = %.2fs total\n", c.TranscribeDurationSec, typeMs, totalDurSec)
	} else if c.TranscribeDurationSec > 0 {
		fmt.Fprintf(w, "Latency:                %.2fs transcribe\n", c.TranscribeDurationSec)
	}
}
