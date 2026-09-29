package sample

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/devsample"
	"ubunatic.com/voxi/spec"
)

// NewCommand creates the top-level sample command.
func NewCommand(d deps.Dependencies) *cobra.Command {
	dataHome := ""
	if d.Getenv != nil {
		dataHome = d.Getenv("XDG_DATA_HOME")
	}
	root := Root(dataHome)
	cmd := &cobra.Command{Use: "sample", Short: "Manage persistent audio samples"}
	open := func() (*Store, error) { return Open(root) }
	var purpose string
	list := &cobra.Command{Use: "list", Short: "List samples in the private store", Long: "List samples in the private store.\n\nExample: voxi sample list --purpose dictation", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		s, err := open()
		if err != nil {
			return err
		}
		var filter []Purpose
		if purpose != "" {
			filter = []Purpose{Purpose(purpose)}
		}
		items, err := s.List(filter...)
		if err != nil {
			return err
		}
		for _, x := range items {
			fmt.Fprintf(d.Stdout, "%s\t%s\t%s\n", x.ID, x.Purpose, x.Transcript)
		}
		return nil
	}}
	list.Flags().StringVar(&purpose, "purpose", "", "limit to dictation, noise, or voice")
	cmd.AddCommand(list)
	cmd.AddCommand(&cobra.Command{Use: "show ID", Short: "Show sample metadata", Long: "Show the metadata for one sample.\n\nExample: voxi sample show greeting", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
		s, err := open()
		if err != nil {
			return err
		}
		x, err := s.Get(a[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(d.Stdout, "id: %s\npurpose: %s\ntranscript: %s\nsource: %s\n", x.ID, x.Purpose, x.Transcript, x.Source)
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "delete ID...", Short: "Delete samples from the private store", Long: "Delete one or more samples from the private store. Every ID is checked before any sample is removed.\n\nExample: voxi sample delete greeting keyboard-noise", Args: cobra.MinimumNArgs(1), RunE: func(c *cobra.Command, a []string) error {
		s, err := open()
		if err != nil {
			return err
		}
		seen := make(map[string]bool, len(a))
		for _, id := range a {
			if _, err := s.Get(id); err != nil {
				return fmt.Errorf("validate sample %q: %w", id, err)
			}
			if seen[id] {
				return fmt.Errorf("duplicate sample id %q", id)
			}
			seen[id] = true
		}
		for _, id := range a {
			if err := s.Delete(id); err != nil {
				return err
			}
		}
		return nil
	}})
	var addPurpose string
	add := &cobra.Command{Use: "add ID", Short: "Add a recent chunk to the sample store", Long: "Copy audio from a recent chunk into the sample store and enter its transcript.\n\nExample: voxi sample add meeting-intro --last --purpose dictation", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
		if addPurpose == string(Voice) {
			return voicePurposeError()
		}
		if addPurpose != string(Dictation) && addPurpose != string(Noise) {
			return fmt.Errorf("invalid sample purpose %q (want dictation or noise)", addPurpose)
		}
		chunkIndex, _ := c.Flags().GetString("chunk")
		last, _ := c.Flags().GetBool("last")
		if chunkIndex == "" && !last {
			return fmt.Errorf("one of --chunk or --last is required")
		}
		if chunkIndex != "" && last {
			return fmt.Errorf("--chunk and --last are mutually exclusive")
		}
		selector := chunkIndex
		if last {
			selector = "last"
		}
		if d.Getenv == nil {
			return fmt.Errorf("chunk storage environment is unavailable")
		}
		b := chunks.NewBuffer(chunks.StorageDir(d.Getenv("XDG_RUNTIME_DIR"), d.Getenv("HOME")), chunks.DefaultBufferSize)
		ch, err := b.Get(selector)
		if err != nil {
			return err
		}
		text := ch.CleanedTranscript
		if text == "" {
			text = ch.RawTranscript
		}
		s, err := open()
		if err != nil {
			return err
		}
		text, err = devsample.PromptTranscript(c.Context(), d, text, func() (string, error) {
			var in *bufio.Reader
			if d.Stdin != nil {
				in = bufio.NewReader(d.Stdin)
			}
			return devsample.PromptText(d.Stdout, in, nil, "Enter the corrected transcript (what you actually said): ", text)
		})
		if err != nil {
			return err
		}
		if text == "" {
			return fmt.Errorf("sample transcript must not be empty")
		}
		return s.Put(Sample{ID: a[0], Purpose: Purpose(addPurpose), Transcript: text, Created: time.Now(), Source: fmt.Sprintf("chunk:%s/%d", ch.SessionID, ch.Index)}, b.WAVPath(ch))
	}}
	add.Flags().String("chunk", "", "chunk index")
	add.Flags().Bool("last", false, "use the most recent chunk")
	add.Flags().StringVar(&addPurpose, "purpose", string(Dictation), "sample purpose: dictation or noise")
	cmd.AddCommand(add)
	var recordPurpose string
	record := &cobra.Command{Use: "record [--purpose P] ID", Short: "Record a sample with a corrected transcript", Long: "Record audio from the microphone and save it with a corrected transcript.\n\nExample: voxi sample record --purpose noise room-tone", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
		if recordPurpose == string(Voice) {
			return voicePurposeError()
		}
		if recordPurpose != string(Dictation) && recordPurpose != string(Noise) {
			return fmt.Errorf("invalid sample purpose %q (want dictation or noise)", recordPurpose)
		}
		return recordSample(c.Context(), d, open, a[0], Purpose(recordPurpose))
	}}
	record.Flags().StringVar(&recordPurpose, "purpose", string(Dictation), "sample purpose: dictation or noise")
	cmd.AddCommand(record)
	cmd.AddCommand(&cobra.Command{Use: "edit ID", Short: "Edit a sample transcript", Long: "Edit the transcript for a sample using $VISUAL or $EDITOR.\n\nExample: voxi sample edit greeting", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
		s, err := open()
		if err != nil {
			return err
		}
		x, err := s.Get(a[0])
		if err != nil {
			return err
		}
		text, err := editTranscript(c.Context(), d, x.Transcript)
		if err != nil {
			return err
		}
		if text == "" {
			return fmt.Errorf("sample transcript must not be empty")
		}
		return s.UpdateTranscript(a[0], text)
	}})
	cmd.AddCommand(&cobra.Command{Use: "move ID PURPOSE", Short: "Move a sample to another purpose", Long: "Move a sample between dictation and noise purposes. Voice moves require consent and are implemented in issue 173.\n\nExample: voxi sample move greeting noise", Args: cobra.ExactArgs(2), RunE: func(c *cobra.Command, a []string) error {
		p := Purpose(a[1])
		if p == Voice {
			return voicePurposeError()
		}
		if p != Dictation && p != Noise {
			return fmt.Errorf("invalid sample purpose %q (want dictation or noise)", p)
		}
		s, err := open()
		if err != nil {
			return err
		}
		return s.Move(a[0], p)
	}})
	cmd.AddCommand(&cobra.Command{Use: "play ID", Short: "Play a sample", Long: "Play a sample using a local audio player.\n\nExample: voxi sample play greeting", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
		s, err := open()
		if err != nil {
			return err
		}
		x, err := s.Get(a[0])
		if err != nil {
			return err
		}
		player, args, err := devsample.PlayerCommand(d)
		if err != nil {
			return err
		}
		if d.Run == nil {
			return fmt.Errorf("audio playback dependencies are unavailable")
		}
		return d.Run(c.Context(), player, args(s.AudioPath(x))...)
	}})
	return cmd
}

func voicePurposeError() error {
	return fmt.Errorf("voice samples require own-voice consent; adding and moving voice samples is implemented in issue 173")
}

func editTranscript(ctx context.Context, d deps.Dependencies, initial string) (string, error) {
	if d.Getenv != nil && strings.EqualFold(strings.TrimSpace(d.Getenv("VOXI_SAMPLE_EDITOR")), "off") {
		return initial, nil
	}
	return devsample.EditTranscript(ctx, devsample.ResolveEditor(d), initial)
}

func recordSample(ctx context.Context, d deps.Dependencies, open func() (*Store, error), id string, purpose Purpose) error {
	if !validID(id) {
		return fmt.Errorf("invalid sample id %q", id)
	}
	s, err := open()
	if err != nil {
		return err
	}
	if exists, err := s.Has(id); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("sample id %q already exists", id)
	}
	if d.Stdin == nil {
		return fmt.Errorf("recording requires stdin for the stop and transcript prompts")
	}
	in := bufio.NewReader(d.Stdin)
	stdinFile, _ := d.Stdin.(*os.File)
	pcm, duration, err := devsample.CaptureUtteranceWithReader(ctx, d, in)
	if err != nil {
		return err
	}
	if d.Stdout != nil {
		fmt.Fprintf(d.Stdout, "Captured %.1fs of audio.\n", duration.Seconds())
	}
	rawTranscript := ""
	if raw, transcribeErr := devsample.TranscribeRawTranscript(ctx, d, pcm); transcribeErr != nil {
		if d.Stdout != nil {
			fmt.Fprintf(d.Stdout, "Note: raw ASR transcript unavailable (%v); starting from a blank transcript.\n", transcribeErr)
		}
	} else {
		rawTranscript = raw
	}
	text, err := devsample.PromptTranscript(ctx, d, rawTranscript, func() (string, error) {
		return devsample.PromptText(d.Stdout, in, stdinFile, "Enter the corrected transcript (what you actually said): ", rawTranscript)
	})
	if err != nil {
		return err
	}
	if text == "" {
		return fmt.Errorf("sample transcript must not be empty")
	}
	var keyterms []string
	if modelSpec, specErr := spec.LoadModels(); specErr != nil {
		if d.Stdout != nil {
			fmt.Fprintf(d.Stdout, "Note: keyterm suggestions unavailable (%v); leaving keyterms empty.\n", specErr)
		}
	} else {
		home := ""
		if d.Getenv != nil {
			home = d.Getenv("HOME")
		}
		suggested := strings.Join(devsample.SuggestKeyterms(text, devsample.CandidateVocabulary(home, modelSpec), modelSpec.SpeechContext.MaxTermChars), "|")
		kt, promptErr := devsample.PromptKeyterms(d.Stdout, in, stdinFile, suggested)
		if promptErr != nil {
			return promptErr
		} else if kt != "" {
			keyterms = strings.Split(kt, "|")
		}
	}
	dir := filepath.Join(s.Root(), string(purpose))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	filename := filepath.Join(dir, ".recording-"+id+".wav")
	defer os.Remove(filename)
	if err := audio.WriteWAVAudio(filename, pcm, devsample.SampleRate); err != nil {
		return err
	}
	if err := os.Chmod(filename, 0o600); err != nil {
		return err
	}
	return s.Put(Sample{ID: id, Purpose: purpose, Transcript: strings.TrimSpace(text), Keyterms: keyterms, Created: time.Now(), Source: "record"}, filename)
}
