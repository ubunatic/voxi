package eager

import (
	"fmt"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/media"
	"ubunatic.com/voxi/internal/tts"
	spec "ubunatic.com/voxi/spec"
)

// newMediaPauser builds the media pauser for one daemon or foreground run
// (issue 125). The session bus address comes from d, never from the process
// environment: with no DBUS_SESSION_BUS_ADDRESS it returns nil, which is a
// valid pauser that does nothing, so tests with a fake environment and hosts
// without a desktop session never touch a media player.
func newMediaPauser(d deps.Dependencies, es *spec.EagerSpec) *media.Pauser {
	if d.Getenv == nil {
		return nil
	}
	address := d.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		return nil
	}
	// voxi's own TTS player is never paused and resumed: a recording stops TTS
	// for good through the playback epoch (issue 152).
	exclude := append([]string{tts.MPRISName}, es.MediaPause.ExcludePlayers...)
	logf := func(string, ...any) {}
	if d.Stdout != nil {
		logf = func(format string, args ...any) {
			fmt.Fprintf(d.Stdout, "voxi eager: "+format+"\n", args...)
		}
	}
	return media.New(media.Options{
		Connect: media.ConnectAddress(address),
		Timeout: es.MediaPauseTimeout(),
		Exclude: exclude,
		Logf:    logf,
	})
}

// sessionMediaPauser is the pauser one capture session uses: p when the
// pause_media setting is on, else nil (a no-op). The setting is read per
// session, so a change applies to the next recording without a daemon restart.
// With no HOME or an unreadable config the default applies.
func sessionMediaPauser(d deps.Dependencies, p *media.Pauser) *media.Pauser {
	if p == nil || d.Getenv == nil {
		return p
	}
	home := d.Getenv("HOME")
	if home == "" {
		return p
	}
	s, err := config.LoadUserSettings(home)
	if err != nil {
		s = config.DefaultUserSettings()
	}
	if !s.PauseMedia {
		return nil
	}
	return p
}
