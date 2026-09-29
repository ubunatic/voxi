package clone

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/devsample"
)

// NewCommand creates the voice management command.
func NewCommand(d deps.Dependencies) *cobra.Command {
	voice := &cobra.Command{Use: "voice", Short: "Prepare and train custom Piper voices"}
	voice.AddCommand(NewPrepareCommand(d), NewTrainCommand(d), NewCloneCommand(d))
	return voice
}

// NewPrepareCommand creates `voxi voice prepare`.
func NewPrepareCommand(d deps.Dependencies) *cobra.Command {
	home := ""
	if d.Getenv != nil {
		home = d.Getenv("HOME")
	}
	samplesDir := devsample.SamplesDir(home)
	outputDir := filepath.Join(home, ".local", "share", "voxi", "voice-training", "dataset")
	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Convert local speech samples to a Piper training dataset",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.LookPath == nil {
				return fmt.Errorf("prepare voice dataset: dependency lookup is unavailable")
			}
			ffmpeg, err := d.LookPath("ffmpeg")
			if err != nil {
				return fmt.Errorf("prepare voice dataset: ffmpeg is required: %w", err)
			}
			run := d.Run
			if run == nil {
				return fmt.Errorf("prepare voice dataset: subprocess runner is unavailable")
			}
			convert := func(ctx context.Context, input, output string) error {
				return run(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", input, "-ar", fmt.Sprint(SampleRate), "-ac", fmt.Sprint(channels), "-c:a", "pcm_s16le", output)
			}
			result, err := Prepare(cmd.Context(), Options{SamplesDir: samplesDir, OutputDir: outputDir, ConvertAudio: convert})
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "prepared %d sample(s) in %s\n", result.Samples, result.OutputDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&samplesDir, "store", samplesDir, "legacy sample directory (until store-backed voice training lands)")
	cmd.Flags().StringVar(&outputDir, "output-dir", outputDir, "directory for the generated LJSpeech dataset")
	cmd.SilenceUsage = true
	return cmd
}
