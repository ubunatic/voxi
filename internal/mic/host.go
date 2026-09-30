package mic

import (
	"context"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

// NewDoctor binds a Doctor to the host: pw-dump for probing, wpctl and
// pw-metadata for repairs.
func NewDoctor(d deps.Dependencies, settle time.Duration) Doctor {
	return Doctor{
		Probe: func(ctx context.Context) (State, error) {
			out, err := d.RunOutput(ctx, "pw-dump")
			if err != nil {
				return State{}, err
			}
			return ParseDump([]byte(out))
		},
		Run:    d.Run,
		Sleep:  d.Sleep,
		Settle: settle,
	}
}

// Notify shows the report as a desktop notification; best effort.
func Notify(ctx context.Context, d deps.Dependencies, rep Report) {
	if d.LookPath == nil || d.Run == nil {
		return
	}
	if _, err := d.LookPath("notify-send"); err != nil {
		return
	}
	_ = d.Run(ctx, "notify-send", "--app-name=Voxi", "--icon=audio-input-microphone", "Voxi", rep.Summary())
}
