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
// Bluetooth keep-awake drop-in, autoswitch-off and headset profile (issue 210).
// --off undoes it; --status reports it without changing anything; --adopt
// takes over a drop-in that someone else wrote.
func newHeadsetCommand(d deps.Dependencies) *cobra.Command {
	var off, status, adopt bool
	cmd := &cobra.Command{
		Use:   "headset",
		Short: "Keep a Bluetooth headset mic awake and stop its switch to call quality (opt-in)",
		Long: "Writes a WirePlumber drop-in that keeps the Bluetooth headset mic link awake between " +
			"dictations, turns off the automatic switch to the headset (HFP) profile, and moves a " +
			"Bluetooth card that is on A2DP to the headset profile, which is the only profile with a mic " +
			"while the switch is off. Costs: the headset mic link stays open while idle (headset battery), " +
			"and headset playback stays at call quality (HFP) until --off.\n\n" +
			"--off removes voxi's drop-in and restores autoswitch only if voxi turned it off. The card " +
			"keeps its headset profile until WirePlumber switches it back. --status only reports. " +
			"--adopt takes over a drop-in that voxi did not write.",
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
			case adopt && (off || status):
				return errors.New("--adopt only applies to enabling")
			case status:
				fmt.Fprintln(d.Stdout, h.StatusLine())
				return nil
			case off:
				return headsetOff(ctx, d, h)
			default:
				return headsetOn(ctx, d, h, adopt)
			}
		},
	}
	cmd.Flags().BoolVar(&off, "off", false, "undo: remove voxi's drop-in and restore autoswitch if voxi turned it off")
	cmd.Flags().BoolVar(&status, "status", false, "show the keep-awake status without changing anything")
	cmd.Flags().BoolVar(&adopt, "adopt", false, "take over a drop-in that is not voxi's and replace it")
	return cmd
}

func headsetOn(ctx context.Context, d deps.Dependencies, h Headset, adopt bool) error {
	res, err := h.Enable(ctx, adopt)
	if err != nil {
		return err
	}
	if res.DropinChanged {
		fmt.Fprintf(d.Stdout, "Wrote %s\n", res.Path)
	} else {
		fmt.Fprintf(d.Stdout, "Drop-in already in place: %s\n", res.Path)
	}
	fmt.Fprintf(d.Stdout, "Autoswitch to the headset profile turned off (%s = false).\n", h.bm.AutoswitchSetting)
	if res.AutoswitchWasOn {
		fmt.Fprintf(d.Stdout, "--off will set it back to true (voxi turned it off).\n")
	} else {
		fmt.Fprintf(d.Stdout, "Autoswitch was already off before voxi ran; --off leaves it off.\n")
	}
	for _, card := range res.PinnedCards {
		fmt.Fprintf(d.Stdout, "Card %s moved to %s (the mic is only available in that profile).\n", card, h.bm.HeadsetProfile)
	}
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
	if res.AutoswitchRestored {
		fmt.Fprintf(d.Stdout, "Autoswitch to the headset profile restored (%s = true).\n", h.bm.AutoswitchSetting)
	} else {
		fmt.Fprintf(d.Stdout, "Autoswitch left as it was (voxi did not turn it off).\n")
	}
	return nil
}
