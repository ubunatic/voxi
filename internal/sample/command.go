package sample

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/devsample"
	"ubunatic.com/voxi/internal/glossary"
	"ubunatic.com/voxi/spec"
)

// NewCommand creates the top-level sample command.
func NewCommand(d deps.Dependencies) *cobra.Command {
	dataHome := ""
	if d.Getenv != nil {
		dataHome = d.Getenv("XDG_DATA_HOME")
	}
	root := Root(dataHome)
	cmd := &cobra.Command{Use: "sample", Short: "Manage persistent audio samples", Long: "Record, save, edit, merge, and publish persistent audio samples.\n\n" + glossary.Audio, SilenceUsage: true}
	open := func() (*Store, error) { return Open(root) }
	publicRoot := DefaultPublicRoot
	cmd.PersistentFlags().StringVar(&publicRoot, "public-store", publicRoot, "public (git-tracked) sample store root, relative to the repository root")
	var purpose string
	var listPublic bool
	list := &cobra.Command{Use: "list", Short: "List samples in the private store", Long: "List samples in the private store.\n\nExample: voxi sample list --purpose dictation", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		s, err := open()
		if listPublic {
			s, err = OpenReadOnly(publicRoot)
		}
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
	list.Flags().BoolVar(&listPublic, "public", false, "list the public store instead of the private store")
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
	var addOwnVoice bool
	add := &cobra.Command{Use: "add ID", Short: "Add a recent chunk to the sample store", Long: "Copy audio from a recent chunk into the sample store and enter its transcript.\n\nExample: voxi sample add meeting-intro --last --purpose dictation", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
		if !validPurpose(Purpose(addPurpose)) {
			return fmt.Errorf("invalid sample purpose %q (want dictation, noise, or voice)", addPurpose)
		}
		in := stdinReader(d)
		var consent *time.Time
		if Purpose(addPurpose) == Voice {
			if err := confirmOwnVoice(d.Stdout, in, addOwnVoice); err != nil {
				return err
			}
			now := time.Now().UTC()
			consent = &now
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
		if text != "" {
			text, err = devsample.PromptTranscript(c.Context(), d, text, func() (string, error) {
				return devsample.PromptText(d.Stdout, in, nil, "Enter the corrected transcript (what you actually said): ", text)
			})
			if err != nil {
				return err
			}
		}
		if Purpose(addPurpose) != Noise && strings.TrimSpace(text) == "" {
			return fmt.Errorf("sample transcript must not be empty")
		}
		return s.Put(Sample{ID: a[0], Purpose: Purpose(addPurpose), Transcript: text, Created: time.Now(), Source: fmt.Sprintf("chunk:%s/%d", ch.SessionID, ch.Index), Consent: consent}, b.WAVPath(ch))
	}}
	add.Flags().String("chunk", "", "chunk index")
	add.Flags().Bool("last", false, "use the most recent chunk")
	add.Flags().StringVar(&addPurpose, "purpose", string(Dictation), "sample purpose: dictation, noise, or voice")
	add.Flags().BoolVar(&addOwnVoice, "own-voice", false, ownVoiceFlagUsage)
	cmd.AddCommand(add)
	var recordPurpose string
	var recordOwnVoice bool
	record := &cobra.Command{Use: "record [--purpose P] ID", Short: "Record a sample with a corrected transcript", Long: "Record audio from the microphone and save it with a corrected transcript.\n\nExample: voxi sample record --purpose noise room-tone", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
		if !validPurpose(Purpose(recordPurpose)) {
			return fmt.Errorf("invalid sample purpose %q (want dictation, noise, or voice)", recordPurpose)
		}
		return recordSample(c.Context(), d, open, a[0], Purpose(recordPurpose), recordOwnVoice)
	}}
	record.Flags().StringVar(&recordPurpose, "purpose", string(Dictation), "sample purpose: dictation, noise, or voice")
	record.Flags().BoolVar(&recordOwnVoice, "own-voice", false, ownVoiceFlagUsage)
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
		if x.Purpose == Dictation && strings.TrimSpace(text) == "" {
			return fmt.Errorf("sample transcript must not be empty")
		}
		return s.UpdateTranscript(a[0], text)
	}})
	var moveOwnVoice bool
	move := &cobra.Command{Use: "move ID PURPOSE", Short: "Move a sample to another purpose", Long: "Move a sample to the dictation, noise, or voice purpose. Moving into voice asks you to confirm that the sample is your own voice and that you consent to cloning it.\n\nExample: voxi sample move greeting noise", Args: cobra.ExactArgs(2), RunE: func(c *cobra.Command, a []string) error {
		p := Purpose(a[1])
		if !validPurpose(p) {
			return fmt.Errorf("invalid sample purpose %q (want dictation, noise, or voice)", p)
		}
		s, err := open()
		if err != nil {
			return err
		}
		x, err := s.Get(a[0])
		if err != nil {
			return err
		}
		if p == Voice && x.Purpose != Voice {
			if strings.TrimSpace(x.Transcript) == "" {
				return fmt.Errorf("sample %q has no transcript; voice samples need one (voxi sample edit %s)", x.ID, x.ID)
			}
			if err := confirmOwnVoice(d.Stdout, stdinReader(d), moveOwnVoice); err != nil {
				return err
			}
			if err := s.GrantConsent(x.ID, time.Now()); err != nil {
				return err
			}
		}
		return s.Move(a[0], p)
	}}
	move.Flags().BoolVar(&moveOwnVoice, "own-voice", false, ownVoiceFlagUsage)
	cmd.AddCommand(move)
	var noSpeech bool
	publish := &cobra.Command{Use: "publish ID", Short: "Publish a noise sample to the public store", Long: "Encode a private noise sample as FLAC and copy it into the public (git-tracked) store. Only noise samples can be published, and you must confirm that the audio contains no intelligible speech.\n\nExample: voxi sample publish keyboard-clack --no-speech", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
		s, err := open()
		if err != nil {
			return err
		}
		x, err := s.Get(a[0])
		if err != nil {
			return err
		}
		if x.Purpose != Noise {
			return fmt.Errorf("sample %q has purpose %s; only noise samples can be published", x.ID, x.Purpose)
		}
		if err := confirm(d.Stdout, stdinReader(d), noSpeech, "This sample contains no intelligible speech"); err != nil {
			return err
		}
		return publishSample(c.Context(), d, s, x, publicRoot)
	}}
	publish.Flags().BoolVar(&noSpeech, "no-speech", false, "confirm without prompting that the sample contains no intelligible speech")
	cmd.AddCommand(publish)
	cmd.AddCommand(newMergeCommand(d, open))
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

// DefaultPublicRoot is the public store inside a voxi checkout.
const DefaultPublicRoot = "testdata/samples"

const ownVoiceFlagUsage = "confirm without prompting that this is your own voice and you consent to cloning it"

func stdinReader(d deps.Dependencies) *bufio.Reader {
	if d.Stdin == nil {
		return nil
	}
	return bufio.NewReader(d.Stdin)
}

func confirmOwnVoice(w io.Writer, in *bufio.Reader, given bool) error {
	return confirm(w, in, given, "This is my own voice and I consent to cloning it")
}

// confirm asks statement as a y/N question unless given is already true.
func confirm(w io.Writer, in *bufio.Reader, given bool, statement string) error {
	if given {
		return nil
	}
	if in == nil {
		return fmt.Errorf("confirmation required: %s", statement)
	}
	if w != nil {
		fmt.Fprintf(w, "%s? [y/N] ", statement)
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("confirmation required: %s", statement)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	return fmt.Errorf("not confirmed: %s", statement)
}

// publishSample FLAC-encodes a private noise sample into the public store.
func publishSample(ctx context.Context, d deps.Dependencies, s *Store, x Sample, publicRoot string) error {
	if d.Run == nil {
		return fmt.Errorf("publish requires ffmpeg; subprocess runner is unavailable")
	}
	pub, err := OpenPublic(publicRoot)
	if err != nil {
		return err
	}
	if exists, err := pub.Has(x.ID); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("sample id %q already exists in the public store %s", x.ID, publicRoot)
	}
	tmp, err := os.MkdirTemp("", "voxi-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	flac := filepath.Join(tmp, x.ID+".flac")
	if err := d.Run(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", s.AudioPath(x), "-compression_level", "12", "-f", "flac", flac); err != nil {
		return fmt.Errorf("flac-encode sample %q: %w", x.ID, err)
	}
	out := Sample{ID: x.ID, Purpose: Noise, Transcript: x.Transcript, Keyterms: x.Keyterms, Created: x.Created, Source: "published:" + x.Source}
	if err := pub.Put(out, flac); err != nil {
		return err
	}
	if d.Stdout != nil {
		fmt.Fprintf(d.Stdout, "published %s to %s\n", x.ID, pub.AudioPath(Sample{Purpose: Noise, Audio: x.ID + ".flac"}))
	}
	return nil
}

func editTranscript(ctx context.Context, d deps.Dependencies, initial string) (string, error) {
	if d.Getenv != nil && strings.EqualFold(strings.TrimSpace(d.Getenv("VOXI_SAMPLE_EDITOR")), "off") {
		return initial, nil
	}
	return devsample.EditTranscript(ctx, devsample.ResolveEditor(d), initial)
}

func recordSample(ctx context.Context, d deps.Dependencies, open func() (*Store, error), id string, purpose Purpose, ownVoice bool) error {
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
	var consent *time.Time
	if purpose == Voice {
		if err := confirmOwnVoice(d.Stdout, in, ownVoice); err != nil {
			return err
		}
		now := time.Now().UTC()
		consent = &now
	}
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
	text := rawTranscript
	if purpose == Noise && rawTranscript == "" {
		if d.Stdout != nil {
			fmt.Fprintln(d.Stdout, "No ASR transcript detected; saving this noise sample with an empty transcript.")
		}
	} else {
		text, err = devsample.PromptTranscript(ctx, d, rawTranscript, func() (string, error) {
			return devsample.PromptText(d.Stdout, in, stdinFile, "Enter the corrected transcript (what you actually said): ", rawTranscript)
		})
		if err != nil {
			return err
		}
	}
	if purpose != Noise && strings.TrimSpace(text) == "" {
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
	return s.Put(Sample{ID: id, Purpose: purpose, Transcript: strings.TrimSpace(text), Keyterms: keyterms, Created: time.Now(), Source: "record", Consent: consent}, filename)
}
