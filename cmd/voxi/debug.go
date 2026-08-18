//go:build debug

package main

import (
	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/debug"
	"ubunatic.com/voxi/internal/deps"
)

func addDebugCommands(root *cobra.Command, d deps.Dependencies) {
	canaryOpts := debug.DefaultCanaryOptions()
	canaryCmd := &cobra.Command{
		Use:   "canary",
		Short: "Interactive real-time streaming canary to observe token delivery and tune parameters",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return debug.RunStreamingCanary(cmd.Context(), d, canaryOpts)
		},
	}
	canaryCmd.Flags().Float64Var(&canaryOpts.ChunkSecs, "chunk", canaryOpts.ChunkSecs, "streaming chunk duration in seconds")
	canaryCmd.Flags().Float64Var(&canaryOpts.LeftContextSecs, "left-context", canaryOpts.LeftContextSecs, "streaming left context in seconds")
	canaryCmd.Flags().Float64Var(&canaryOpts.RightContextSecs, "right-context", canaryOpts.RightContextSecs, "streaming right context in seconds")
	canaryCmd.Flags().BoolVar(&canaryOpts.VAD, "vad", canaryOpts.VAD, "enable voice activity detection (VAD)")
	canaryCmd.Flags().Float64Var(&canaryOpts.VADThreshold, "vad-threshold", canaryOpts.VADThreshold, "VAD speech threshold")
	canaryCmd.Flags().StringVar(&canaryOpts.VADBackend, "vad-backend", canaryOpts.VADBackend, "VAD backend")

	vadOpts := debug.DefaultVADProbeOptions()
	vadCmd := &cobra.Command{
		Use:   "vad-probe",
		Short: "Prototype VAD-segmented sentence-by-sentence dictation with audio pre-roll buffer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return debug.RunVADProbe(cmd.Context(), d, vadOpts)
		},
	}
	vadCmd.Flags().IntVar(&vadOpts.ThresholdRMS, "threshold", vadOpts.ThresholdRMS, "audio RMS energy threshold")
	vadCmd.Flags().IntVar(&vadOpts.SilenceMs, "silence", vadOpts.SilenceMs, "silence duration in ms")
	vadCmd.Flags().IntVar(&vadOpts.PreRollMs, "pre-roll", vadOpts.PreRollMs, "pre-speech circular buffer duration in ms")
	vadCmd.Flags().IntVar(&vadOpts.MinSpeechMs, "min-speech", vadOpts.MinSpeechMs, "minimum speech duration in ms")
	vadCmd.Flags().BoolVar(&vadOpts.TypeOutput, "type", vadOpts.TypeOutput, "type transcribed sentences into focused window")

	root.AddCommand(canaryCmd, vadCmd)
}
