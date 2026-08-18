package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/eager"
	"ubunatic.com/voxi/internal/history"
	"ubunatic.com/voxi/internal/mode"
	"ubunatic.com/voxi/internal/modifiers"
	"ubunatic.com/voxi/internal/monitor"
	"ubunatic.com/voxi/internal/record"
	"ubunatic.com/voxi/internal/typing"
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
	eagerCmd.Flags().StringVar(&eagerOpts.Model, "model", eagerOpts.Model, "Whisper model name (default: small.en, or base.en)")

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

	root.AddCommand(modeCmd, recordCmd, eagerCmd, monitorCmd, historyCmd, configCmd, daemonCmd)
	addDebugCommands(root, d)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
