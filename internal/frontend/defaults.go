// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

package frontend

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/godbus/dbus/v5"

	"ubunatic.com/voxi/internal/agent"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/tts"
	"ubunatic.com/voxi/spec"
)

// mprisName is the session-bus name voxi's TTS player registers (see internal/tts/mpris.go).
const mprisName = "org.mpris.MediaPlayer2.voxi"

// NewDeps wires the real agent socket, TTS socket, commands, HTTP probe and
// host media check for the user whose home directory is home.
func NewDeps(home string) Deps {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	return Deps{
		Home:     home,
		Agent:    agent.DefaultClient(),
		TTS:      tts.Client{SocketPath: tts.SocketPath(runtimeDir, os.Getuid())},
		Run:      execRunner,
		Probe:    httpProbe,
		Models:   spec.LoadModels,
		Settings: config.LoadUserSettings,
		Media:    hostMedia,
	}
}

// execRunner runs a command without a shell. A missing binary returns an error
// that wraps exec.ErrNotFound.
func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// httpProbe sends one GET and discards the body. It only reads status.
func httpProbe(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, nil
}

// hostMedia reports whether the OS media controls can reach voxi's player.
// Only Linux has a read-only check here. macOS and Windows need native APIs
// (cgo or WinRT), which this build does not include.
func hostMedia(ctx context.Context) MediaStatus {
	switch runtime.GOOS {
	case "linux":
		return mprisStatus(ctx)
	case "darwin":
		return MediaStatus{Status: "not built for macOS", Detail: "macOS media controls need the MediaPlayer framework; not in this build"}
	case "windows":
		return MediaStatus{Status: "not built for Windows", Detail: "Windows media controls need SystemMediaTransportControls; not in this build"}
	default:
		return MediaStatus{Status: "unsupported OS", Detail: "no host media integration for " + runtime.GOOS}
	}
}

// mprisStatus checks, read-only, whether voxi's MPRIS name is on the session bus.
func mprisStatus(ctx context.Context) MediaStatus {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return MediaStatus{Status: "unavailable", Detail: "no session bus: " + err.Error()}
	}
	defer func() { _ = conn.Close() }()
	var owned bool
	bus := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus")
	if err := bus.CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, mprisName).Store(&owned); err != nil {
		return MediaStatus{Status: "unavailable", Detail: "session bus query failed: " + err.Error()}
	}
	if !owned {
		return MediaStatus{Status: "unavailable", Detail: mprisName + " is not on the session bus"}
	}
	return MediaStatus{Available: true, Status: "available", Detail: mprisName + " is on the session bus"}
}
