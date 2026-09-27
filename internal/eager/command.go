package eager

import "github.com/spf13/cobra"

// NewCommand builds the eager command and its flags. execute may be nil when
// a caller only needs the canonical flag parser.
func NewCommand(execute func(*cobra.Command, []string) error, opts *EagerOptions) *cobra.Command {
	if opts == nil {
		defaults := DefaultEagerOptions()
		opts = &defaults
	}
	cmd := &cobra.Command{
		Use:   "eager",
		Short: "Continuous eager sentence streaming dictation into focused window",
		Args:  cobra.NoArgs,
	}
	if execute != nil {
		cmd.RunE = execute
	}
	cmd.Flags().IntVar(&opts.ThresholdRMS, "threshold", opts.ThresholdRMS, "audio RMS energy threshold to trigger speech detection (default: 150)")
	cmd.Flags().IntVar(&opts.SilenceMs, "silence", opts.SilenceMs, "silence duration in ms to finalize an utterance chunk (default: 800)")
	cmd.Flags().IntVar(&opts.PreRollMs, "pre-roll", opts.PreRollMs, "pre-speech circular buffer duration in ms to preserve starting phonemes (default: 500)")
	cmd.Flags().IntVar(&opts.MinSpeechMs, "min-speech", opts.MinSpeechMs, "minimum speech duration in ms to ignore noise (default: 200)")
	cmd.Flags().IntVar(&opts.MaxWindowMs, "max-window", opts.MaxWindowMs, "maximum window length in ms before forcing a phrase chunk (default: 8000)")
	cmd.Flags().BoolVar(&opts.TypeOutput, "type", opts.TypeOutput, "type transcribed sentences directly into the focused window via dotool")
	cmd.Flags().BoolVar(&opts.RecordHistory, "history", opts.RecordHistory, "record transcribed utterances into local dictation history")
	cmd.Flags().BoolVar(&opts.ModifierGating, "modifier-gating", opts.ModifierGating, "wait for physical modifiers before typing output")
	cmd.Flags().BoolVar(&opts.Daemon, "daemon", opts.Daemon, "run as background systemd daemon listening for toggle control")
	cmd.Flags().StringVar(&opts.Model, "model", opts.Model, "model name, see spec/models.yaml")
	cmd.Flags().BoolVar(&opts.SpeechContext, "speech-context", opts.SpeechContext, "bounded local vocabulary hints")
	cmd.Flags().StringSliceVar(&opts.Vocabulary, "vocabulary", opts.Vocabulary, "additional comma-separated speech-context terms")
	return cmd
}
