package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/agent"
	"ubunatic.com/voxi/internal/bench"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/eager"
	"ubunatic.com/voxi/internal/feedback"
	"ubunatic.com/voxi/internal/history"
	"ubunatic.com/voxi/internal/mode"
	"ubunatic.com/voxi/internal/modifiers"
	"ubunatic.com/voxi/internal/monitor"
	"ubunatic.com/voxi/internal/record"
	"ubunatic.com/voxi/internal/telemetry"
	"ubunatic.com/voxi/internal/typing"
	"ubunatic.com/voxi/spec"
)

func main() {
	d := deps.DefaultDependencies(os.Stdin, os.Stdout)

	root := &cobra.Command{
		Use:   "voxi",
		Short: "Standalone Linux voice input, continuous eager streaming, and desktop typing engine",
		Long: "Voxi is a high-performance voice input and typing engine for Linux/Wayland.\n" +
			"It provides continuous eager sentence streaming with rolling Whisper inference,\n" +
			"modifier key gating daemon for hotkey safety, synthetic typing injection, and a btop-style monitor.",
	}

	// 1. mode command
	modeCmd := &cobra.Command{
		Use:   "mode [batch|streaming|eager]",
		Short: "Show or switch the active voice-input mode",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if len(args) == 0 {
				if status, err := agent.DefaultClient().Status(ctx); err == nil {
					fmt.Fprintf(d.Stdout, "%s (voxi agent, %s)\n", status.Mode, status.Recording)
					return nil
				} else if !errors.Is(err, agent.ErrUnavailable) {
					return fmt.Errorf("get agent voice-input mode: %w", err)
				}
				fmt.Fprintln(d.Stdout, mode.DescribeVoiceInputMode(mode.CurrentVoiceInputMode(ctx, d)))
				return nil
			}
			var target mode.VoiceInputMode
			switch args[0] {
			case "streaming":
				target = mode.ModeStreaming
			case "batch":
				target = mode.ModeBatch
			case "eager":
				target = mode.ModeEager
			default:
				return fmt.Errorf("invalid mode %q (want batch, streaming, or eager)", args[0])
			}
			return mode.SwitchVoiceInputMode(ctx, d, target)
		},
	}

	// 2. record command
	recordCmd := &cobra.Command{
		Use:   "record",
		Short: "Start, stop, or toggle active voice dictation recording",
	}
	toggleCmd := &cobra.Command{
		Use:   "toggle",
		Short: "Toggle dictation recording on or off",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return record.ControlRecording(cmd.Context(), d, record.RecordActionToggle)
		},
	}
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start dictation recording",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return record.ControlRecording(cmd.Context(), d, record.RecordActionStart)
		},
	}
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop dictation recording",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return record.ControlRecording(cmd.Context(), d, record.RecordActionStop)
		},
	}
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Print recording status (idle or recording)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			stat, err := record.GetRecordingStatus(cmd.Context(), d)
			if err != nil {
				return err
			}
			fmt.Fprintln(d.Stdout, stat)
			return nil
		},
	}
	recordCmd.AddCommand(toggleCmd, startCmd, stopCmd, statusCmd)

	// 3. eager command
	eagerOpts := eager.DefaultEagerOptions()
	eagerCmd := &cobra.Command{
		Use:   "eager",
		Short: "Continuous eager sentence streaming dictation into focused window",
		Long: "Continuously captures audio from the microphone with a circular pre-roll buffer.\n" +
			"Segments speech on natural conversational pauses (silence > 800ms) or rolling windows,\n" +
			"transcribes completed phrases immediately with local Whisper, and types finalized sentences\n" +
			"directly into the active application via dotool with zero dropped words across pauses.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return eager.RunEagerDictation(cmd.Context(), d, eagerOpts)
		},
	}
	eagerCmd.Flags().IntVar(&eagerOpts.ThresholdRMS, "threshold", eagerOpts.ThresholdRMS, "audio RMS energy threshold to trigger speech detection (default: 150)")
	eagerCmd.Flags().IntVar(&eagerOpts.SilenceMs, "silence", eagerOpts.SilenceMs, "silence duration in ms to finalize an utterance chunk (default: 800)")
	eagerCmd.Flags().IntVar(&eagerOpts.PreRollMs, "pre-roll", eagerOpts.PreRollMs, "pre-speech circular buffer duration in ms to preserve starting phonemes (default: 500)")
	eagerCmd.Flags().IntVar(&eagerOpts.MinSpeechMs, "min-speech", eagerOpts.MinSpeechMs, "minimum speech duration in ms to ignore noise (default: 200)")
	eagerCmd.Flags().IntVar(&eagerOpts.MaxWindowMs, "max-window", eagerOpts.MaxWindowMs, "maximum window length in ms before forcing a phrase chunk (default: 8000)")
	eagerCmd.Flags().BoolVar(&eagerOpts.TypeOutput, "type", eagerOpts.TypeOutput, "type transcribed sentences directly into the focused window via dotool")
	eagerCmd.Flags().BoolVar(&eagerOpts.RecordHistory, "history", eagerOpts.RecordHistory, "record transcribed utterances into local dictation history")
	eagerCmd.Flags().BoolVar(&eagerOpts.Daemon, "daemon", eagerOpts.Daemon, "run as background systemd daemon listening for toggle control")
	eagerCmd.Flags().StringVar(&eagerOpts.Model, "model", eagerOpts.Model, fmt.Sprintf("Whisper model name, see spec/models.yaml (default: %s)", eagerOpts.Model))
	eagerCmd.Flags().BoolVar(&eagerOpts.SpeechContext, "speech-context", eagerOpts.SpeechContext, "bounded local vocabulary hints for small.en (default: on; use --speech-context=false to disable)")
	eagerCmd.Flags().StringSliceVar(&eagerOpts.Vocabulary, "vocabulary", eagerOpts.Vocabulary, "additional comma-separated speech-context terms (requires --speech-context)")

	// 4. monitor / top / resources command
	var watch bool
	var intervalSec int
	var sectionsStr string
	monitorCmd := &cobra.Command{
		Use:     "monitor",
		Aliases: []string{"top", "resources", "stats"},
		Short:   "Monitor voice input daemon, GPU acceleration, memory, and subprocess resources",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			sections := monitor.ParseSections(sectionsStr)
			if watch {
				return monitor.RunWatchResources(ctx, d, time.Duration(intervalSec)*time.Second, sections)
			}
			report := monitor.CollectVoiceResources(ctx, d)
			monitor.PrintVoiceResourceReport(d.Stdout, report, sections)
			return nil
		},
	}
	monitorCmd.Flags().BoolVarP(&watch, "watch", "w", false, "continuously refresh resource metrics")
	monitorCmd.Flags().IntVarP(&intervalSec, "interval", "i", 1, "refresh interval in seconds for --watch")
	monitorCmd.Flags().StringVarP(&sectionsStr, "sections", "s", "all", "comma-separated sections: s(speed), h(hardware), t(transcript), d(daemons)")

	// 5. history command
	historyPath := func() string { return history.HistoryPath(d.Getenv("HOME")) }
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Recent dictation history (local-only, treat as sensitive)",
	}
	var histLimit int
	var histFormat string
	histList := &cobra.Command{
		Use:   "list",
		Short: "List recent transcripts, most recent first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := history.ListHistory(historyPath())
			if err != nil {
				return err
			}
			if histLimit > 0 && len(entries) > histLimit {
				entries = entries[:histLimit]
			}
			if histFormat == "json" {
				if entries == nil {
					entries = []history.HistoryEntry{}
				}
				enc := json.NewEncoder(d.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}
			for _, e := range entries {
				fmt.Fprintf(d.Stdout, "%s\t%s\t%s\n", e.ID, e.Time.Format("2006-01-02T15:04:05"), e.Text)
			}
			return nil
		},
	}
	histList.Flags().IntVar(&histLimit, "limit", 0, "show at most N entries (default: all)")
	histList.Flags().StringVar(&histFormat, "format", "text", "output format (text or json)")

	histClear := &cobra.Command{
		Use:   "clear",
		Short: "Delete all recorded history",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return history.ClearHistory(historyPath())
		},
	}
	histRecord := &cobra.Command{
		Use:   "record",
		Short: "Internal: record stdin as a history entry and echo it back unchanged",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := io.ReadAll(d.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			text := string(data)
			if _, err := history.AppendHistory(historyPath(), text, history.DefaultHistoryLimit, time.Now()); err != nil {
				return err
			}
			_, err = fmt.Fprint(d.Stdout, text)
			return err
		},
	}
	histCopy := &cobra.Command{
		Use:   "copy ID",
		Short: "Copy a history entry's text to the clipboard",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := history.FindHistoryEntry(historyPath(), args[0])
			if err != nil {
				return err
			}
			return typing.CopyText(cmd.Context(), d, entry.Text)
		},
	}
	histRetype := &cobra.Command{
		Use:   "retype ID",
		Short: "Type a history entry's text into the currently focused window",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := history.FindHistoryEntry(historyPath(), args[0])
			if err != nil {
				return err
			}
			return typing.TypeText(cmd.Context(), d, entry.Text)
		},
	}
	historyCmd.AddCommand(histList, histClear, histRecord, histCopy, histRetype)

	// 6. config command
	cfgPath := func() string { return config.VoxtypeConfigPath(d.Getenv("HOME")) }
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Read or change select voxtype config.toml fields",
	}
	cfgGet := &cobra.Command{
		Use:   "get type-delay-ms",
		Short: "Print current type_delay_ms value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "type-delay-ms" {
				return fmt.Errorf("unknown config key %q (want type-delay-ms)", args[0])
			}
			ms, ok, err := config.ReadTypeDelayMs(cfgPath())
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(d.Stdout, "0 (default; key not present in config.toml)")
				return nil
			}
			fmt.Fprintln(d.Stdout, ms)
			return nil
		},
	}
	cfgSet := &cobra.Command{
		Use:   "set type-delay-ms MS",
		Short: "Set type_delay_ms, preserving comments and other settings",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "type-delay-ms" {
				return fmt.Errorf("unknown config key %q (want type-delay-ms)", args[0])
			}
			ms, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid milliseconds %q: %w", args[1], err)
			}
			if err := config.SetTypeDelayMs(cfgPath(), ms); err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "type_delay_ms set to %d in %s.\n"+
				"This does not take effect until the daemon restarts:\n"+
				"  systemctl --user restart voxtype.service            # batch mode\n"+
				"  systemctl --user restart voxtype-streaming.service   # streaming mode\n"+
				"  systemctl --user restart voxi-eager.service          # eager mode\n",
				ms, cfgPath())
			return nil
		},
	}
	configCmd.AddCommand(cfgGet, cfgSet)

	// 7. daemon command
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run background services managed by voxi",
	}
	daemonCmd.AddCommand(modifiers.NewModifierDaemonCommand(d))

	// 8. bench command
	benchOpts := bench.DefaultOptions()
	var benchJSONPath string
	benchCmd := &cobra.Command{
		Use:   "bench",
		Short: "Benchmark CPU vs GPU transcription speed across configured Whisper models",
		Long: "Transcribes one audio clip through every model in spec/models.yaml on each\n" +
			"requested backend, reporting the real-time factor (RTF) and speedup for each\n" +
			"model x backend pair. By default it uses a fixed reference clip downloaded\n" +
			"on demand into the user cache dir (never committed to the repo); pass\n" +
			"--record for a live microphone clip or --file for a local WAV.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := bench.Run(cmd.Context(), d, benchOpts)
			if err != nil {
				return err
			}
			printBenchReport(d.Stdout, report)
			if benchJSONPath != "" {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return fmt.Errorf("encode bench report: %w", err)
				}
				if err := os.WriteFile(benchJSONPath, data, 0644); err != nil {
					return fmt.Errorf("write %s: %w", benchJSONPath, err)
				}
				fmt.Fprintf(d.Stdout, "\nWrote %s\n", benchJSONPath)
			}
			return nil
		},
	}
	benchCmd.Flags().BoolVar(&benchOpts.Record, "record", benchOpts.Record, "record live microphone audio instead of using the downloaded reference clip")
	benchCmd.Flags().IntVar(&benchOpts.DurationSecs, "duration", benchOpts.DurationSecs, "seconds of microphone audio to record when --record is set")
	benchCmd.Flags().StringVar(&benchOpts.WavFile, "file", benchOpts.WavFile, "use this 16kHz mono WAV instead of the reference clip or recording")
	benchCmd.Flags().StringSliceVar(&benchOpts.Models, "models", benchOpts.Models, "comma-separated model names to bench (default: every model in spec/models.yaml)")
	benchCmd.Flags().StringSliceVar(&benchOpts.Backends, "backends", benchOpts.Backends, fmt.Sprintf("comma-separated backends to bench: cpu, gpu (default: %s)", strings.Join(benchOpts.Backends, ",")))
	benchCmd.Flags().IntVar(&benchOpts.Threads, "threads", benchOpts.Threads, "CPU threads passed to voxtype")
	benchCmd.Flags().StringVar(&benchJSONPath, "json", "", "write the full bench report as JSON to this path")

	modelSpec, err := spec.LoadModels()
	if err != nil {
		panic(fmt.Sprintf("load embedded model specification: %v", err))
	}
	root.AddCommand(modeCmd, recordCmd, eagerCmd, monitorCmd, historyCmd, configCmd, daemonCmd, benchCmd, telemetry.NewCommand(d.Stdout, d.Getenv), feedback.NewCommand(d.Stdout, d.Getenv("HOME"), modelSpec.BuiltinStopWords(modelSpec.DefaultModel), modelSpec.SpeechContext.MaxTermChars, modelSpec.SpeechContext.Terms, d), agent.NewCommand(d), chunks.NewCommand(d, nil))
	addDebugCommands(root, d)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// printBenchReport renders a CPU-vs-GPU RTF/speedup table grouped by model.
func printBenchReport(w io.Writer, report *bench.Report) {
	fmt.Fprintf(w, "\nAudio: %.2fs (%s)\n", report.AudioSecs, report.AudioSource)
	fmt.Fprintf(w, "%-20s %-10s %8s %10s %s\n", "MODEL", "BACKEND", "RTF", "SPEEDUP", "DETECTED")
	for _, r := range report.Results {
		if r.Error != "" {
			fmt.Fprintf(w, "%-20s %-10s %8s %10s %s\n", r.Model, r.Backend, "-", "-", "error: "+r.Error)
			continue
		}
		fmt.Fprintf(w, "%-20s %-10s %8.2f %9.2fx %s\n", r.Model, r.Backend, r.RTF, r.Speedup, r.DetectedBackend)
	}
}
