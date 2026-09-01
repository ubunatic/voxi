package feedback

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/spec"
)

// NewCommand creates the local feedback command. home is injected to keep the
// CLI testable and to avoid relying on a global process home directory.
func NewCommand(out io.Writer, home string, builtins []spec.StopWord) *cobra.Command {
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
		disabled := map[string]bool{}
		for _, id := range o.Disabled {
			disabled[id] = true
		}
		for _, r := range builtins {
			state := "built-in"
			if disabled[r.ID] {
				state = "disabled"
			}
			fmt.Fprintf(out, "%s\t%s\t%s\n", state, r.ID, r.Pattern)
		}
		for _, p := range o.User {
			fmt.Fprintf(out, "user\t-\t%s\n", p)
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
	cmd.AddCommand(stop)
	return cmd
}
