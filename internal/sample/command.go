package sample

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
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
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
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
	cmd.AddCommand(&cobra.Command{Use: "show ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
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
	cmd.AddCommand(&cobra.Command{Use: "delete ID...", Args: cobra.MinimumNArgs(1), RunE: func(c *cobra.Command, a []string) error {
		s, err := open()
		if err != nil {
			return err
		}
		for _, id := range a {
			if err := s.Delete(id); err != nil {
				return err
			}
		}
		return nil
	}})
	export := &cobra.Command{Use: "export", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		tsv, _ := c.Flags().GetBool("tsv")
		if !tsv {
			return fmt.Errorf("--tsv is required")
		}
		s, err := open()
		if err != nil {
			return err
		}
		items, err := s.List()
		if err != nil {
			return err
		}
		_, err = d.Stdout.Write(ExportTSV(items))
		return err
	}}
	export.Flags().Bool("tsv", false, "write legacy corpus.tsv format")
	cmd.AddCommand(export)
	add := &cobra.Command{Use: "add ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
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
		return s.Put(Sample{ID: a[0], Purpose: Dictation, Transcript: text, Created: time.Now(), Source: "chunk:" + selector}, b.WAVPath(ch))
	}}
	add.Flags().String("chunk", "", "chunk index")
	add.Flags().Bool("last", false, "use the most recent chunk")
	cmd.AddCommand(add)
	cmd.AddCommand(&cobra.Command{Use: "play ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
		s, err := open()
		if err != nil {
			return err
		}
		x, err := s.Get(a[0])
		if err != nil {
			return err
		}
		if d.LookPath == nil || d.Run == nil {
			return fmt.Errorf("audio playback dependencies are unavailable")
		}
		player, err := d.LookPath("paplay")
		if err != nil {
			return err
		}
		return d.Run(c.Context(), player, s.AudioPath(x))
	}})
	return cmd
}
