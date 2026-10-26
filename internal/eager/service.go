package eager

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/modifiers"
	"ubunatic.com/voxi/internal/telemetry"
	"ubunatic.com/voxi/internal/typing"
	spec "ubunatic.com/voxi/spec"
)

// eagerSocketTimeout bounds a client's round trip to the eager daemon's
// control socket (start/stop/toggle/status). See issue 057: this is a
// safety net, not the primary fix -- the primary fix (eagerSessionManager)
// keeps the daemon side from ever blocking a request behind a stale
// transcription drain, so this deadline should not normally be reached.
const eagerSocketTimeout = 8 * time.Second

// DaemonService provides control and status queries for the running eager daemon.
type DaemonService interface {
	Control(ctx context.Context, action string) error
	Status(ctx context.Context) (string, error)
}

// DefaultDaemonService implements DaemonService by communicating over the local UNIX domain socket.
type DefaultDaemonService struct {
	Deps deps.Dependencies
}

// Control sends an action (start, stop, toggle) to the daemon.
func (s DefaultDaemonService) Control(ctx context.Context, action string) error {
	return ControlEagerDaemon(ctx, s.Deps, action)
}

// Status returns current recording or idle status.
func (s DefaultDaemonService) Status(ctx context.Context) (string, error) {
	return GetEagerRecordingStatus(ctx, s.Deps)
}

func runEagerDaemon(ctx context.Context, d deps.Dependencies, opts EagerOptions) error {
	runtimeDir := d.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sockDir := filepath.Join(runtimeDir, "voxi")
	_ = os.MkdirAll(sockDir, 0755)
	sockPath := filepath.Join(sockDir, "eager.sock")
	_ = os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix socket %s: %w", sockPath, err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(sockPath)
	}()

	recCmdName := ""
	var recArgs []string
	if _, err := d.LookPath("pw-record"); err == nil {
		recCmdName = "pw-record"
		recArgs = []string{"--rate", "16000", "--channels", "1", "--format", "s16", "-"}
	} else if _, err := d.LookPath("arecord"); err == nil {
		recCmdName = "arecord"
		recArgs = []string{"-r", "16000", "-c", "1", "-f", "S16_LE", "-t", "raw", "-q", "-"}
	} else {
		return fmt.Errorf("audio capture tool (pw-record or arecord) missing")
	}

	if err := checkEagerModelReady(ctx, d, opts); err != nil {
		return err
	}
	go typing.WatchInputSource(ctx, d, 150*time.Millisecond)

	tmpDir, err := os.MkdirTemp(d.Getenv("XDG_RUNTIME_DIR"), "voxi-eager-daemon-*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	writeVoxtypeState("idle")
	defer writeVoxtypeState("inactive")

	eagerSpec, err := spec.LoadEager()
	if err != nil {
		return fmt.Errorf("eager: load eager spec: %w", err)
	}

	recorder := telemetry.NewRecorder(telemetry.Path(d.Getenv("XDG_DATA_HOME"), d.Getenv("HOME")))
	var sessions *eagerSessionManager
	sessions = newEagerSessionManager(ctx, recorder, func(sessCtx context.Context, sessionID string, request *stopRequest, onCaptureStopped func()) {
		// Each session gets its own subdirectory under the daemon's base
		// tmpDir rather than sharing one across sessions: since Stop/Start
		// no longer wait for the previous session's transcription drain to
		// finish (issue 057 fix), a new session's capture can now begin
		// while the previous one is still draining in the background, and
		// both name their per-utterance WAV files starting at utt_001 --
		// sharing a directory would let a fresh session's audio collide
		// with (or be clobbered by) a stale drain still reading/writing
		// under the same names.
		sessTmpDir, err := os.MkdirTemp(tmpDir, "sess-*")
		if err != nil {
			sessTmpDir = tmpDir
		} else {
			defer os.RemoveAll(sessTmpDir)
		}
		// Write "idle" the moment capture actually stops, not only once this
		// whole session (including any still-draining transcription) fully
		// returns -- otherwise external state readers (voxi monitor, the
		// GNOME extension) would keep reporting "recording" for up to
		// transcribeTimeout after the user told the daemon to stop.
		_ = runEagerCaptureSessionAt(sessCtx, d, opts, sessTmpDir, recCmdName, recArgs, true, sessionID, request, recorder, func() {
			writeVoxtypeState("idle")
			onCaptureStopped()
		}, sessions)
	})
	sessions.modifierStartGrace = eagerSpec.ModifierStartGrace()

	// Modifier-release race guard (issue 101): a lightweight poller feeds
	// eagerSessionManager.NoteModifierPress whenever any watched physical
	// modifier reads active, so modifierBuffer.EnterIfNeeded can judge
	// staleness against a recent press. Any watched modifier counts for a
	// first cut (narrowing to Super-only, if Ctrl/Alt/Shift use proves too
	// noisy in practice, is a one-line change to the mask check below, not
	// an architectural one). Polls at the same 10ms cadence
	// modifiers.WaitModifiersReleased already uses.
	if opts.ModifierGating {
		go func() {
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if active, _ := modifiers.AreModifiersActive(); active {
						sessions.NoteModifierPress(time.Now())
					}
				}
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for sig := range sigChan {
			if sig == syscall.SIGUSR1 {
				sessions.Toggle()
			} else if sig == syscall.SIGUSR2 {
				sessions.Stop()
			}
		}
	}()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
		sessions.Stop()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			scanner := bufio.NewScanner(c)
			if scanner.Scan() {
				cmd := strings.TrimSpace(scanner.Text())
				switch cmd {
				case "toggle":
					msg := sessions.Toggle()
					_, _ = fmt.Fprintln(c, msg)
				case "start":
					sessions.Start()
					_, _ = fmt.Fprintln(c, "Recording started")
				case "stop":
					sessions.Stop()
					_, _ = fmt.Fprintln(c, "Recording stopped")
				case "status":
					if sessions.Recording() {
						_, _ = fmt.Fprintln(c, "recording")
					} else {
						_, _ = fmt.Fprintln(c, "idle")
					}
				default:
					_, _ = fmt.Fprintln(c, "unknown command")
				}
			}
		}(conn)
	}

	// Wait for every session's full lifetime (capture + transcription drain)
	// to finish before returning, so the deferred os.RemoveAll(tmpDir) above
	// never yanks the directory out from under a still-draining background
	// transcription, and so a final queued utterance still gets typed before
	// the daemon process exits.
	sessions.Wait()

	return nil
}

// ControlEagerDaemon sends recording control actions to the eager daemon socket.
func ControlEagerDaemon(ctx context.Context, d deps.Dependencies, action string) error {
	runtimeDir := d.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sockPath := filepath.Join(runtimeDir, "voxi", "eager.sock")
	if _, err := os.Stat(sockPath); err != nil {
		// Try legacy path
		legacySock := filepath.Join(runtimeDir, "harnez", "eager.sock")
		if _, err := os.Stat(legacySock); err == nil {
			sockPath = legacySock
		}
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("eager daemon not reachable at %s (is voxi-eager.service active?): %w", sockPath, err)
	}
	defer conn.Close()
	// Defense-in-depth bound on the eager-daemon socket round trip, mirroring
	// the transcribeTimeout precedent: with the issue 057 fix, start/stop/
	// toggle now return as soon as capture stops rather than blocking on a
	// stale transcription drain, so this should virtually never fire. It
	// exists so any future daemon-side stall degrades to a clear error
	// instead of silently wedging the caller (internal/agent/agent.go's
	// Agent.Record holds a single mutex across the whole backend call, so an
	// indefinite block here previously stalled every other agent request).
	_ = conn.SetDeadline(time.Now().Add(eagerSocketTimeout))

	_, err = fmt.Fprintf(conn, "%s\n", action)
	if err != nil {
		return fmt.Errorf("send %s to eager daemon: %w", action, err)
	}

	res, _ := bufio.NewReader(conn).ReadString('\n')
	if strings.TrimSpace(res) != "" {
		fmt.Fprintln(d.Stdout, strings.TrimSpace(res))
	}
	return nil
}

// GetEagerRecordingStatus queries current recording/idle status from the eager daemon.
func GetEagerRecordingStatus(ctx context.Context, d deps.Dependencies) (string, error) {
	runtimeDir := d.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sockPath := filepath.Join(runtimeDir, "voxi", "eager.sock")
	if _, err := os.Stat(sockPath); err != nil {
		legacySock := filepath.Join(runtimeDir, "harnez", "eager.sock")
		if _, err := os.Stat(legacySock); err == nil {
			sockPath = legacySock
		}
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return "inactive", nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(eagerSocketTimeout))

	_, _ = fmt.Fprintln(conn, "status")
	res, _ := bufio.NewReader(conn).ReadString('\n')
	status := strings.TrimSpace(res)
	if status == "" {
		return "idle", nil
	}
	return status, nil
}
