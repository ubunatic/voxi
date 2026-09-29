package clone

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/sample"
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
	dataHome := ""
	if d.Getenv != nil {
		dataHome = d.Getenv("XDG_DATA_HOME")
	}
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	storeRoot := sample.Root(dataHome)
	outputDir := filepath.Join(home, ".local", "share", "voxi", "voice-training", "dataset")
	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Convert local speech samples to a Piper training dataset",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			warnLegacyAllowlist(d.Stderr, home)
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
			result, err := Prepare(cmd.Context(), Options{StoreRoot: storeRoot, OutputDir: outputDir, ConvertAudio: convert})
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "prepared %d sample(s) in %s\n", result.Samples, result.OutputDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&storeRoot, "store", storeRoot, "private sample store root; only voice-purpose samples are used")
	cmd.Flags().StringVar(&outputDir, "output-dir", outputDir, "directory for the generated LJSpeech dataset")
	cmd.SilenceUsage = true
	return cmd
}

// warnLegacyAllowlist notes that the pre-store allowlist no longer selects
// training samples; only the store's voice/ folder does.
func warnLegacyAllowlist(w io.Writer, home string) {
	legacy := filepath.Join(home, ".config", "voxi", "samples", "voice-training.txt")
	if _, err := os.Stat(legacy); err == nil && w != nil {
		fmt.Fprintf(w, "note: %s is ignored; only voice-purpose samples in the sample store are used\n", legacy)
	}
}
