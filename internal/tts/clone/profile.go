package clone

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/sample"
)

// VoicesDir is where cloned-voice reference WAVs are installed (issue 155 M2).
func VoicesDir(home string) string {
	return filepath.Join(home, ".local", "share", "voxi", "voices")
}

// NewCloneCommand creates `voxi voice clone`, which installs a reference WAV
// from voice-purpose store data as a cloned-voice profile for a future engine
// backend and records it in ~/.config/voxi/config.yaml.
//
// Consent: the voice profile is local and private, and is intended to hold
// only the user's own recorded voice, tracked through sample-store consent.
func NewCloneCommand(d deps.Dependencies) *cobra.Command {
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
	sampleID := ""
	name := "cloned"
	cmd := &cobra.Command{
		Use:   "clone",
		Short: "Install a voice sample as a cloned-voice profile",
		Long: "Copies one voice-purpose sample WAV from the private sample store to " +
			"~/.local/share/voxi/voices/<name>.wav and records it as " +
			"tts_voice_reference_wav in ~/.config/voxi/config.yaml for a cloned-voice engine. " +
			"Only clone your own voice.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !safeID.MatchString(name) {
				return fmt.Errorf("invalid voice name %q: use 1-128 ASCII letters, digits, underscores, or hyphens; start with a letter or digit", name)
			}
			store, err := sample.OpenReadOnly(storeRoot)
			if err != nil {
				return fmt.Errorf("open sample store: %w", err)
			}
			kept, err := store.List(sample.Voice)
			if err != nil {
				return err
			}
			if len(kept) == 0 {
				return fmt.Errorf("no voice-purpose samples in %s", storeRoot)
			}
			sample, err := selectSample(kept, sampleID)
			if err != nil {
				return err
			}
			input := store.AudioPath(sample)
			resolvedInput, err := filepath.EvalSymlinks(input)
			if err != nil {
				return fmt.Errorf("sample %q WAV %q: %w", sample.ID, sample.Audio, err)
			}
			resolvedSamples, err := filepath.EvalSymlinks(storeRoot)
			if err != nil {
				return fmt.Errorf("resolve sample store: %w", err)
			}
			if !within(resolvedSamples, resolvedInput) {
				return fmt.Errorf("sample %q WAV path resolves outside the sample store", sample.ID)
			}
			voiceDir := VoicesDir(home)
			if err := os.MkdirAll(voiceDir, 0700); err != nil {
				return fmt.Errorf("create voice profile directory: %w", err)
			}
			target := filepath.Join(voiceDir, name+".wav")
			if err := copyReplacing(resolvedInput, target); err != nil {
				return fmt.Errorf("install voice profile: %w", err)
			}
			if err := config.SetTTSVoiceReferenceWav(home, target); err != nil {
				return fmt.Errorf("record tts_voice_reference_wav: %w", err)
			}
			fmt.Fprintf(d.Stdout, "installed cloned-voice profile %s from sample %q at %s\n", name, sample.ID, target)
			return nil
		},
	}
	cmd.Flags().StringVar(&storeRoot, "store", storeRoot, "private sample store root; only voice-purpose samples are used")
	cmd.Flags().StringVar(&sampleID, "sample", sampleID, "voice sample id to clone (required when more than one voice sample exists)")
	cmd.Flags().StringVar(&name, "name", name, "voice profile name used for the installed WAV")
	cmd.SilenceUsage = true
	return cmd
}

func selectSample(kept []sample.Sample, sampleID string) (sample.Sample, error) {
	if sampleID == "" {
		if len(kept) == 1 {
			return kept[0], nil
		}
		var ids []string
		for _, s := range kept {
			ids = append(ids, s.ID)
		}
		slices.Sort(ids)
		return sample.Sample{}, fmt.Errorf("multiple voice-purpose samples available; choose one with --sample: %s", strings.Join(ids, ", "))
	}
	for _, item := range kept {
		if item.ID == sampleID {
			return item, nil
		}
	}
	return sample.Sample{}, fmt.Errorf("sample %q is not a voice-purpose sample", sampleID)
}

// copyReplacing copies source to destination, replacing any existing file
// atomically via a staged temporary file in the destination directory.
func copyReplacing(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".voice-clone-*.wav")
	if err != nil {
		return err
	}
	staging := tmp.Name()
	defer os.Remove(staging)
	if _, err := io.Copy(tmp, input); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(staging, 0600); err != nil {
		return err
	}
	return os.Rename(staging, destination)
}
