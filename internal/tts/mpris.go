package tts

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	mprisName = "org.mpris.MediaPlayer2.voxi"
	mprisPath = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	playerIF  = "org.mpris.MediaPlayer2.Player"
	rootIF    = "org.mpris.MediaPlayer2"
)

// MPRIS owns the session-bus name for the lifetime of the monitor watch.
type MPRIS struct {
	conn      *dbus.Conn
	props     *prop.Properties
	mu        sync.Mutex
	closeOnce sync.Once
	closeErr  error
	cancel    context.CancelFunc
}

// StartMPRIS registers the monitor-owned queue as a desktop media player.
func StartMPRIS(ctx context.Context, manager *Manager) (*MPRIS, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to session D-Bus for MPRIS: %w", err)
	}
	cleanup := func(err error) (*MPRIS, error) {
		conn.Close()
		return nil, err
	}
	result, err := conn.RequestName(mprisName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return cleanup(fmt.Errorf("request MPRIS name: %w", err))
	}
	if result != dbus.RequestNameReplyPrimaryOwner {
		return cleanup(fmt.Errorf("MPRIS name %s is already owned", mprisName))
	}
	serviceCtx, cancel := context.WithCancel(ctx)
	service := &MPRIS{conn: conn, cancel: cancel}
	methods := map[string]any{
		"PlayPause":   func() *dbus.Error { return service.control(manager, ActionPlayPause) },
		"Play":        func() *dbus.Error { return service.resume(manager) },
		"Pause":       func() *dbus.Error { return service.pause(manager) },
		"Previous":    func() *dbus.Error { return service.control(manager, ActionPrevious) },
		"Next":        func() *dbus.Error { return service.control(manager, ActionNext) },
		"Stop":        func() *dbus.Error { return service.control(manager, ActionStop) },
		"Seek":        func(int64) *dbus.Error { return nil },
		"SetPosition": func(dbus.ObjectPath, int64) *dbus.Error { return nil },
		"OpenUri":     func(string) *dbus.Error { return nil },
	}
	if err := conn.ExportMethodTable(methods, mprisPath, playerIF); err != nil {
		return cleanup(fmt.Errorf("export MPRIS player methods: %w", err))
	}
	rootMethods := map[string]any{
		"Raise": func() *dbus.Error { return nil },
		"Quit":  func() *dbus.Error { return nil },
	}
	if err := conn.ExportMethodTable(rootMethods, mprisPath, rootIF); err != nil {
		return cleanup(fmt.Errorf("export MPRIS root methods: %w", err))
	}
	service.props, err = prop.Export(conn, mprisPath, mprisProperties())
	if err != nil {
		return cleanup(fmt.Errorf("export MPRIS properties: %w", err))
	}
	intro := &introspect.Node{Interfaces: mprisIntrospection()}
	if err := conn.Export(introspect.NewIntrospectable(intro), mprisPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return cleanup(fmt.Errorf("export MPRIS introspection: %w", err))
	}
	service.update(manager.Snapshot())
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-serviceCtx.Done():
				_ = service.Close()
				return
			case <-ticker.C:
				service.update(manager.Snapshot())
			}
		}
	}()
	return service, nil
}

func mprisProperties() prop.Map {
	props := prop.Map{
		rootIF: {
			"CanQuit": {Value: false}, "CanRaise": {Value: false}, "HasTrackList": {Value: false},
			"Identity": {Value: "Voxi"}, "DesktopEntry": {Value: "voxi"},
			"SupportedUriSchemes": {Value: []string{}}, "SupportedMimeTypes": {Value: []string{}},
		},
		playerIF: {
			"PlaybackStatus": {Value: "Stopped", Emit: prop.EmitTrue}, "LoopStatus": {Value: "None"},
			"Rate": {Value: 1.0}, "Shuffle": {Value: false},
			"Metadata": {Value: map[string]dbus.Variant{}}, "Volume": {Value: 1.0}, "Position": {Value: int64(0)},
			"MinimumRate": {Value: 1.0}, "MaximumRate": {Value: 1.0},
			"CanGoNext": {Value: true}, "CanGoPrevious": {Value: true}, "CanPlay": {Value: true},
			"CanPause": {Value: true}, "CanSeek": {Value: false}, "CanControl": {Value: true},
		},
	}
	return props
}

func (m *MPRIS) control(manager *Manager, action Action) *dbus.Error {
	if err := manager.Control(action); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

func (m *MPRIS) pause(manager *Manager) *dbus.Error {
	return m.control(manager, ActionPause)
}

func (m *MPRIS) resume(manager *Manager) *dbus.Error {
	return m.control(manager, ActionResume)
}

func (m *MPRIS) update(snapshot Snapshot) {
	status := "Stopped"
	if snapshot.Status == statusPlaying {
		status = "Playing"
	} else if snapshot.Status == statusPaused {
		status = "Paused"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.props.Set(playerIF, "PlaybackStatus", dbus.MakeVariant(status))
	metadata := map[string]dbus.Variant{}
	if snapshot.Current != "" {
		metadata["xesam:title"] = dbus.MakeVariant(snapshot.Current)
		metadata["xesam:artist"] = dbus.MakeVariant([]string{"Voxi"})
		metadata["mpris:trackid"] = dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/track/current"))
	}
	_ = m.props.Set(playerIF, "Metadata", dbus.MakeVariant(metadata))
}

func mprisIntrospection() []introspect.Interface {
	properties := mprisProperties()
	rootProps := introspectionProperties(properties[rootIF])
	playerProps := introspectionProperties(properties[playerIF])
	return []introspect.Interface{
		{Name: rootIF, Methods: []introspect.Method{{Name: "Raise"}, {Name: "Quit"}}, Properties: rootProps},
		{Name: playerIF, Methods: []introspect.Method{
			{Name: "Next"}, {Name: "Previous"}, {Name: "Pause"}, {Name: "PlayPause"}, {Name: "Stop"},
			{Name: "Play"}, {Name: "Seek", Args: []introspect.Arg{{Name: "Offset", Type: "x", Direction: "in"}}},
			{Name: "SetPosition", Args: []introspect.Arg{{Name: "TrackId", Type: "o", Direction: "in"}, {Name: "Position", Type: "x", Direction: "in"}}},
			{Name: "OpenUri", Args: []introspect.Arg{{Name: "Uri", Type: "s", Direction: "in"}}},
		}, Properties: playerProps},
		prop.IntrospectData,
		introspect.IntrospectData,
	}
}

func introspectionProperties(properties map[string]*prop.Prop) []introspect.Property {
	result := make([]introspect.Property, 0, len(properties))
	for name, property := range properties {
		result = append(result, property.Introspection(name))
	}
	return result
}

// Close releases the well-known name and D-Bus connection.
func (m *MPRIS) Close() error {
	if m == nil || m.conn == nil {
		return nil
	}
	m.closeOnce.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		_, _ = m.conn.ReleaseName(mprisName)
		m.closeErr = m.conn.Close()
	})
	return m.closeErr
}
