package mic

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// newHeadsetCommand creates `voxi mic headset`: the explicit opt-in for the
// Bluetooth keep-awake drop-in and autoswitch-off (issue 210). --off undoes it;
// --status reports it without changing anything.
func newHeadsetCommand(d deps.Dependencies) *cobra.Command {
	var off, status bool
	cmd := &cobra.Command{
		Use:   "headset",
		Short: "Keep a Bluetooth headset mic awake and stop its switch to call quality (opt-in)",
		Long: "Writes a WirePlumber drop-in that keeps the Bluetooth headset mic link awake between " +
			"dictations, and turns off the automatic switch to the headset (HFP) profile, so playback " +
			"stays at high quality. Costs: the headset mic link stays open while idle (headset battery), " +
			"and playback stays at call quality while autoswitch is off.\n\n" +
			"--off removes voxi's drop-in and restores autoswitch. --status only reports.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			es, err := spec.LoadEager()
			if err != nil {
				return err
			}
			h := NewHeadset(d, es.BluetoothMic)
			switch {
			case off && status:
				return errors.New("--off and --status cannot be combined")
			case status:
				fmt.Fprintln(d.Stdout, h.StatusLine())
				return nil
			case off:
				return headsetOff(ctx, d, h)
			default:
				return headsetOn(ctx, d, h)
			}
		},
	}
	cmd.Flags().BoolVar(&off, "off", false, "undo: remove voxi's drop-in and restore autoswitch")
	cmd.Flags().BoolVar(&status, "status", false, "show the keep-awake status without changing anything")
	return cmd
}

func headsetOn(ctx context.Context, d deps.Dependencies, h Headset) error {
	res, err := h.Enable(ctx)
	if err != nil {
		return err
	}
	if res.DropinChanged {
		fmt.Fprintf(d.Stdout, "Wrote %s\n", res.Path)
	} else {
		fmt.Fprintf(d.Stdout, "Drop-in already in place: %s\n", res.Path)
	}
	fmt.Fprintln(d.Stdout, "Autoswitch to the headset profile turned off (bluetooth.autoswitch-to-headset-profile = false).")
	fmt.Fprintln(d.Stdout, "Headset playback now stays at call quality, and the headset mic link stays open while idle (headset battery).")
	fmt.Fprintln(d.Stdout, "WirePlumber reads drop-ins when it starts. If the idle headset still suspends after a 30 s pause,")
	fmt.Fprintln(d.Stdout, "restart it (this interrupts audio): systemctl --user restart wireplumber")
	fmt.Fprintln(d.Stdout, "Undo: voxi mic headset --off")
	return nil
}

func headsetOff(ctx context.Context, d deps.Dependencies, h Headset) error {
	res, err := h.Disable(ctx)
	if err != nil {
		return err
	}
	if res.Removed {
		fmt.Fprintf(d.Stdout, "Removed %s\n", h.DropinPath())
	} else {
		fmt.Fprintln(d.Stdout, "No voxi drop-in to remove.")
	}
	fmt.Fprintln(d.Stdout, "Autoswitch to the headset profile restored (bluetooth.autoswitch-to-headset-profile = true).")
	return nil
}
