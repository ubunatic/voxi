package feedback

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/devsample"
	"ubunatic.com/voxi/internal/listing"
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
// sampleTranscribe transcribes samples for `sample list --process` (issue
// 098). It is passed in explicitly (rather than imported directly from
// internal/eager) to avoid an import cycle: internal/eager already imports
// internal/feedback. A nil value disables --process with a clear error
// rather than a panic.
func NewCommand(out io.Writer, home string, builtins []spec.StopWord, maxVocabularyTermChars int, staticVocabularyTerms []string, d deps.Dependencies, sampleTranscribe SampleTranscribeFunc) *cobra.Command {
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
			fmt.Fprintf(out, "Added vocabulary term %q. It is active for Whisper small.en on the next `voxi eager` run; the default Cohere backend does not use decoder vocabulary prompting. Disable prompting with --speech-context=false\n", term)
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
	replacementPath := ReplacementPath(home)
	replacement := &cobra.Command{Use: "replacement", Short: "Manage exact Cohere transcript corrections"}
	replacement.AddCommand(
		func() *cobra.Command {
			add := &cobra.Command{Use: "add HEARD WRITTEN", Short: "Add a heard-form to written-form mapping, case-insensitive and case-adaptive", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, a []string) error {
				fixedCase, _ := cmd.Flags().GetBool("fixed-case")
				rules, err := LoadReplacements(replacementPath)
				if err != nil {
					return err
				}
				rules, rule, err := AddReplacement(rules, a[0], a[1], fixedCase)
				if err != nil {
					return err
				}
				if err := SaveReplacements(replacementPath, rules); err != nil {
					return err
				}
				note := ""
				if fixedCase {
					note = " (fixed case: always written exactly as given)"
				}
				fmt.Fprintf(out, "Added Cohere transcript replacement %q -> %q%s. Remove it with: voxi feedback replacement remove %q\n", rule.From, rule.To, note, rule.From)
				return nil
			}}
			add.Flags().Bool("fixed-case", false, "always write WRITTEN exactly as given, regardless of how HEARD was cased (use for domains and the like)")
			return add
		}(),
		&cobra.Command{Use: "list", Short: "List exact Cohere transcript corrections", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
			rules, err := LoadReplacements(replacementPath)
			if err != nil {
				return err
			}
			for _, rule := range rules {
				if rule.FixedCase {
					fmt.Fprintf(out, "%s\t%s\t[fixed-case]\n", rule.From, rule.To)
				} else {
					fmt.Fprintf(out, "%s\t%s\n", rule.From, rule.To)
				}
			}
			return nil
		}},
		&cobra.Command{Use: "remove HEARD", Short: "Remove an exact heard-form mapping", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, a []string) error {
			rules, err := LoadReplacements(replacementPath)
			if err != nil {
				return err
			}
			rules, rule, err := RemoveReplacement(rules, a[0])
			if err != nil {
				return err
			}
			if err := SaveReplacements(replacementPath, rules); err != nil {
				return err
			}
			fmt.Fprintf(out, "Removed Cohere transcript replacement %q -> %q. Add it again with: voxi feedback replacement add %q %q\n", rule.From, rule.To, rule.From, rule.To)
			return nil
		}},
		func() *cobra.Command {
			cleanup := &cobra.Command{
				Use:   "cleanup",
				Short: "Remove case-variant duplicates now redundant under case-insensitive matching",
				Args:  cobra.NoArgs,
				RunE: func(cmd *cobra.Command, _ []string) error {
					dryRun, _ := cmd.Flags().GetBool("dry-run")
					rules, err := LoadReplacements(replacementPath)
					if err != nil {
						return err
					}
					kept, dropped := DedupeReplacements(rules)
					if len(dropped) == 0 {
						fmt.Fprintln(out, "No duplicate replacements found.")
						return nil
					}
					for _, d := range dropped {
						if d.Conflict {
							fmt.Fprintf(out, "Dropping %q -> %q: conflicts with kept %q -> %q (its target wins; the dropped target is lost)\n", d.Dropped.From, d.Dropped.To, d.Kept.From, d.Kept.To)
						} else {
							fmt.Fprintf(out, "Dropping %q -> %q: case variant of kept %q\n", d.Dropped.From, d.Dropped.To, d.Kept.From)
						}
					}
					if dryRun {
						fmt.Fprintf(out, "Dry run: %d of %d replacements would be removed. Re-run without --dry-run to apply.\n", len(dropped), len(rules))
						return nil
					}
					if err := SaveReplacements(replacementPath, kept); err != nil {
						return err
					}
					fmt.Fprintf(out, "Removed %d duplicate replacement(s); %d remain.\n", len(dropped), len(kept))
					return nil
				},
			}
			cleanup.Flags().Bool("dry-run", false, "preview duplicates without writing changes")
			return cleanup
		}(),
	)
	cmd.AddCommand(replacement)
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
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recorded dev samples (private, and public/promoted with --all)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, _ := cmd.Flags().GetBool("all")
			full, _ := cmd.Flags().GetBool("full")
			short, _ := cmd.Flags().GetBool("short")
			process, _ := cmd.Flags().GetBool("process")
			color, _ := cmd.Flags().GetString("color")
			if short && full {
				return fmt.Errorf("--full and --short are mutually exclusive")
			}
			if short {
				full = false
			}
			return runSampleList(cmd.Context(), out, d, home, sampleListOptions{
				All:     all,
				Full:    full,
				Process: process,
				Color:   color,
			}, sampleTranscribe)
		},
	}
	listCmd.Flags().Bool("all", false, "include the public/promoted corpus (testdata/noise-samples) alongside the private one")
	listCmd.Flags().Bool("full", false, "show a richer, chunks-list-style table (timestamp, on-the-fly duration/RMS/sparkline, full transcript)")
	listCmd.Flags().Bool("short", false, "show the compact table (default; explicit form of the default)")
	listCmd.Flags().Bool("process", false, "also run each listed sample through the cohere-transcribe engine and show the fresh transcript next to the stored ground truth (requires --full)")
	listCmd.Flags().String("color", listing.ColorAuto, "colorize the LEVEL sparkline by loudness in --full output: auto, always, or never")
	sample.AddCommand(
		recordCmd,
		listCmd,
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
		func() *cobra.Command {
			saveChunk := func(cmd *cobra.Command, selector, name string) error {
				force, _ := cmd.Flags().GetBool("force")
				chunkBuf := chunks.NewBuffer(chunks.StorageDir(d.Getenv("XDG_RUNTIME_DIR"), d.Getenv("HOME")), chunks.DefaultBufferSize)
				chunk, err := chunkBuf.Get(selector)
				if err != nil {
					return fmt.Errorf("retrieve chunk %s: %w", selector, err)
				}
				wavPath := chunkBuf.WAVPath(chunk)
				defaultText := chunk.CleanedTranscript
				if defaultText == "" {
					defaultText = chunk.RawTranscript
				}
				return devsample.SaveChunkAsSample(cmd.Context(), d, home, name, wavPath, defaultText, force)
			}

			saveChunkCmd := &cobra.Command{
				Use:   "save-chunk [INDEX] NAME",
				Short: "Save a recorded audio chunk from the ring buffer into the sample library",
				Args:  cobra.RangeArgs(1, 2),
				RunE: func(cmd *cobra.Command, a []string) error {
					selector := "last"
					name := a[0]
					if len(a) == 2 {
						selector = a[0]
						name = a[1]
					}
					return saveChunk(cmd, selector, name)
				},
			}
			saveChunkCmd.Flags().Bool("force", false, "overwrite an existing sample without confirmation")

			saveLastCmd := &cobra.Command{
				Use:   "save-last NAME",
				Short: "Save the most recent recorded audio chunk into the sample library",
				Args:  cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, a []string) error {
					return saveChunk(cmd, "last", a[0])
				},
			}
			saveLastCmd.Flags().Bool("force", false, "overwrite an existing sample without confirmation")

			sample.AddCommand(saveChunkCmd)
			return saveLastCmd
		}(),
	)

	promoteCmd := &cobra.Command{
		Use:   "promote NAME",
		Short: "Move a noise-only private sample into the public, git-tracked corpus (testdata/noise-samples)",
		Long: "Move a private dev sample into the public, git-tracked corpus at testdata/noise-samples,\n" +
			"FLAC-encoding its audio for git-lfs. Only promote samples confirmed to contain no real\n" +
			"speech (keyboard/mouse/ambient noise) -- this command does not and cannot verify that.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			publicDir, _ := cmd.Flags().GetString("to")
			if err := devsample.Promote(cmd.Context(), home, publicDir, a[0]); err != nil {
				return err
			}
			fmt.Fprintf(out, "Promoted sample %q to %s\n", a[0], publicDir)
			return nil
		},
	}
	promoteCmd.Flags().String("to", devsample.PublicSamplesDir(""), "public samples directory to promote into (run from the repo root)")
	sample.AddCommand(promoteCmd)

	importCmd := &cobra.Command{
		Use:   "import PATH",
		Short: "Merge samples from another machine's local sample directory into this one",
		Long: "Merge every sample listed in PATH's corpus.tsv-compatible manifest into this\n" +
			"machine's private sample library, copying each referenced WAV. PATH must already\n" +
			"be a local directory (e.g. copied over with scp/rsync/USB beforehand) -- import\n" +
			"performs no transfer of its own.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			overwrite, _ := cmd.Flags().GetBool("overwrite")
			_, err := devsample.Import(home, a[0], overwrite, out)
			return err
		},
	}
	importCmd.Flags().Bool("overwrite", false, "replace an existing local sample with the same name instead of skipping it")
	sample.AddCommand(importCmd)

	cmd.AddCommand(sample)

	return cmd
}

// NewConfigImportCommand builds the `voxi config import DIR` command,
// merging stop-words, replacements, vocabulary, and dev samples from
// another machine's ~/.config/voxi-shaped directory into this one. It is
// wired as a subcommand of voxi's top-level `config` command (cmd/voxi's
// existing voxtype-config.toml command) rather than a competing top-level
// "config" command, since cobra already owns that name; the resulting CLI
// shape is still exactly `voxi config import DIR`.
//
// config.yaml and env are deliberately not covered here -- they are
// machine-specific (paths, device IDs) in ways stop-words/replacements/
// vocabulary/samples aren't, so a blind merge risks importing settings that
// don't apply to the target machine (see issue 118 and 119).
func NewConfigImportCommand(out io.Writer, home string, maxVocabularyTermChars int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import DIR",
		Short: "Merge stop-words, replacements, vocabulary, and dev samples from another machine's local Voxi state",
		Long: "Merge stop-words.json, replacements.json, vocabulary.txt, and samples/ from DIR --\n" +
			"a local, previously-copied ~/.config/voxi-shaped directory -- into this machine's\n" +
			"local Voxi state. DIR must already be a local directory (e.g. copied over with\n" +
			"scp/rsync/USB beforehand) -- import performs no transfer of its own.\n\n" +
			"config.yaml and env are intentionally not imported: they hold machine-specific\n" +
			"settings (paths, device IDs) that don't safely carry over between machines.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			overwrite, _ := cmd.Flags().GetBool("overwrite")
			only, _ := cmd.Flags().GetStringSlice("only")
			return Import(home, a[0], ImportOptions{Overwrite: overwrite, Only: only}, maxVocabularyTermChars, out)
		},
	}
	cmd.Flags().Bool("overwrite", false, "replace a colliding replacement entry instead of skipping it (stop-words, vocabulary, and samples are always additive/skip-on-collision)")
	cmd.Flags().StringSlice("only", nil, "import only these areas: "+strings.Join(AreaNames, ",")+" (default: all found in DIR)")
	return cmd
}
