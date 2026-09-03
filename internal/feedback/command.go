package feedback

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/devsample"
	"ubunatic.com/voxi/internal/speechcontext"
	"ubunatic.com/voxi/spec"
)

// NewCommand creates the local feedback command. home is injected to keep the
// CLI testable and to avoid relying on a global process home directory.
// staticVocabularyTerms is the shipped spec/models.yaml speech-context term
// list, reported (as a count) by `voxi feedback status`. d is used only for
// the `sample` subcommands, which need real microphone capture / audio
// playback and stdin prompting; the rest of this command tree stays
// deps-free by design.
func NewCommand(out io.Writer, home string, builtins []spec.StopWord, maxVocabularyTermChars int, staticVocabularyTerms []string, d deps.Dependencies) *cobra.Command {
	path := Path(home)
	load := func() (Overrides, error) { return Load(path) }
	cmd := &cobra.Command{Use: "feedback", Short: "Manage local dictation feedback"}
	stop := &cobra.Command{Use: "stop-word", Short: "Manage local ASR stop-word rules"}
	add := &cobra.Command{Use: "add PHRASE", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, a []string) error {
		o, err := load()
		if err != nil {
			return err
		}
		o, err = Add(o, a[0])
		if err != nil {
			return err
		}
		if err = Save(path, o); err != nil {
			return err
		}
		fmt.Fprintf(out, "Added user stop word %q. Remove it with: voxi feedback stop-word remove %q\n", a[0], a[0])
		return nil
	}}
	remove := &cobra.Command{Use: "remove PHRASE", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, a []string) error {
		o, err := load()
		if err != nil {
			return err
		}
		o, err = Remove(o, a[0])
		if err != nil {
			return err
		}
		if err = Save(path, o); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed user stop word %q. Add it again with: voxi feedback stop-word add %q\n", a[0], a[0])
		return nil
	}}
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		o, err := load()
		if err != nil {
			return err
		}
		for _, r := range StopWordRows(o, builtins) {
			fmt.Fprintf(out, "%s\t%s\t%s\n", r.State, r.ID, r.Pattern)
		}
		return nil
	}}
	set := func(enabled bool) *cobra.Command {
		verb := "disable"
		reverse := "enable"
		if enabled {
			verb, reverse = reverse, verb
		}
		return &cobra.Command{Use: verb + " ID", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, a []string) error {
			o, err := load()
			if err != nil {
				return err
			}
			o, err = SetBuiltin(o, a[0], enabled, builtins)
			if err != nil {
				return err
			}
			if err = Save(path, o); err != nil {
				return err
			}
			fmt.Fprintf(out, "%sd built-in rule %q. Reverse with: voxi feedback stop-word %s %s\n", map[bool]string{true: "Enable", false: "Disable"}[enabled], a[0], reverse, a[0])
			return nil
		}}
	}
	stop.AddCommand(add, list, remove, set(false), set(true))
	artifact := &cobra.Command{Use: "silence-artifact", Short: "Manage whole-utterance silence artifacts"}
	artifact.AddCommand(
		&cobra.Command{Use: "add PHRASE", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, a []string) error {
			o, err := load()
			if err != nil {
				return err
			}
			o, err = AddSilenceArtifact(o, a[0])
			if err != nil {
				return err
			}
			if err = Save(path, o); err != nil {
				return err
			}
			fmt.Fprintf(out, "Added silence artifact %q. It is discarded only as a whole utterance. Remove it with: voxi feedback silence-artifact remove %q\n", a[0], a[0])
			return nil
		}},
		&cobra.Command{Use: "remove PHRASE", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, a []string) error {
			o, err := load()
			if err != nil {
				return err
			}
			o, err = RemoveSilenceArtifact(o, a[0])
			if err != nil {
				return err
			}
			if err = Save(path, o); err != nil {
				return err
			}
			fmt.Fprintf(out, "Removed silence artifact %q. Add it again with: voxi feedback silence-artifact add %q\n", a[0], a[0])
			return nil
		}},
		&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
			o, err := load()
			if err != nil {
				return err
			}
			for _, p := range o.SilenceArtifacts {
				fmt.Fprintf(out, "silence-artifact\t%s\n", p)
			}
			return nil
		}},
	)
	cmd.AddCommand(stop)
	cmd.AddCommand(artifact)
	vocabularyPath := speechcontext.VocabularyPath(home)
	vocabulary := &cobra.Command{Use: "vocabulary", Short: "Manage persistent speech-context terms"}
	vocabulary.AddCommand(
		&cobra.Command{Use: "add TERM", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, a []string) error {
			terms, err := speechcontext.LoadVocabulary(vocabularyPath, maxVocabularyTermChars)
			if err != nil {
				return err
			}
			terms, term, err := speechcontext.AddVocabulary(terms, a[0], maxVocabularyTermChars)
			if err != nil {
				return err
			}
			if err := speechcontext.SaveVocabulary(vocabularyPath, terms, maxVocabularyTermChars); err != nil {
				return err
			}
			fmt.Fprintf(out, "Added vocabulary term %q. It is active by default on the next `voxi eager` run (small.en); disable prompting with --speech-context=false\n", term)
			return nil
		}},
		&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
			terms, err := speechcontext.LoadVocabulary(vocabularyPath, maxVocabularyTermChars)
			if err != nil {
				return err
			}
			for _, term := range terms {
				fmt.Fprintln(out, term)
			}
			return nil
		}},
		&cobra.Command{Use: "remove TERM", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, a []string) error {
			terms, err := speechcontext.LoadVocabulary(vocabularyPath, maxVocabularyTermChars)
			if err != nil {
				return err
			}
			terms, term, err := speechcontext.RemoveVocabulary(terms, a[0], maxVocabularyTermChars)
			if err != nil {
				return err
			}
			if err := speechcontext.SaveVocabulary(vocabularyPath, terms, maxVocabularyTermChars); err != nil {
				return err
			}
			fmt.Fprintf(out, "Removed vocabulary term %q. Add it again with: voxi feedback vocabulary add %q\n", term, term)
			return nil
		}},
	)
	cmd.AddCommand(vocabulary)
	status := &cobra.Command{
		Use:   "status",
		Short: "Show a combined summary of local stop-word, silence-artifact, and vocabulary state",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			o, err := load()
			if err != nil {
				return err
			}
			terms, err := speechcontext.LoadVocabulary(vocabularyPath, maxVocabularyTermChars)
			if err != nil {
				return err
			}
			explicitPresent := false
			if _, statErr := os.Stat(vocabularyPath); statErr == nil {
				explicitPresent = true
			}
			sources := VocabularySources{
				ExplicitFilePresent: explicitPresent,
				ExplicitFileTerms:   len(terms),
				StaticSpecTerms:     len(staticVocabularyTerms),
			}
			// --speech-context is an eager flag that defaults to true (issue 046);
			// there is no persisted override for it, so this reports the
			// built-in default rather than a per-invocation choice.
			summary := BuildSummary(o, builtins, terms, sources, true)
			return FormatSummary(out, summary)
		},
	}
	cmd.AddCommand(status)

	sample := &cobra.Command{Use: "sample", Short: "Manage private local dev/testing speech samples (not used by dictation)"}
	recordCmd := &cobra.Command{
		Use:   "record NAME",
		Short: "Record one microphone utterance and its manually corrected transcript",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			force, _ := cmd.Flags().GetBool("force")
			return devsample.Record(cmd.Context(), d, home, a[0], force)
		},
	}
	recordCmd.Flags().Bool("force", false, "overwrite an existing sample without confirmation")
	sample.AddCommand(
		recordCmd,
		&cobra.Command{
			Use:   "list",
			Short: "List recorded dev samples",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				samples, err := devsample.LoadManifest(home)
				if err != nil {
					return err
				}
				if len(samples) == 0 {
					fmt.Fprintln(out, "No dev samples recorded yet. Record one with: voxi feedback sample record <name>")
					return nil
				}
				for _, s := range samples {
					fmt.Fprintf(out, "%s\t%s\t%s\n", s.Name, s.Timestamp.Local().Format("2006-01-02 15:04:05"), s.Preview(60))
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "play NAME",
			Short: "Replay a recorded dev sample",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, a []string) error {
				return devsample.Play(cmd.Context(), d, home, a[0])
			},
		},
		&cobra.Command{
			Use:   "remove NAME",
			Short: "Delete a recorded dev sample and its transcript",
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, a []string) error {
				if err := devsample.Remove(home, a[0]); err != nil {
					return err
				}
				fmt.Fprintf(out, "Removed sample %q\n", a[0])
				return nil
			},
		},
	)
	cmd.AddCommand(sample)

	return cmd
}
