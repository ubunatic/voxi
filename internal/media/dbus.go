package media

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/godbus/dbus/v5"
)

// MPRIS protocol names (https://specifications.freedesktop.org/mpris-spec/latest/).
const (
	busPrefix  = "org.mpris.MediaPlayer2."
	objectPath = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	playerIF   = "org.mpris.MediaPlayer2.Player"
)

// sessionBus is the real Bus: a private session D-Bus connection for one pass.
type sessionBus struct {
	conn *dbus.Conn
}

// connectSessionBus opens a private connection that also closes when ctx ends,
// so a call to a hung player cannot outlive the pass budget.
func connectSessionBus(ctx context.Context) (Bus, error) {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("connect to session D-Bus: %w", err)
	}
	return &sessionBus{conn: conn}, nil
}

// ConnectAddress returns an Options.Connect that opens a private connection to
// the bus at address (the value of DBUS_SESSION_BUS_ADDRESS). Callers that take
// the environment from injected dependencies use it instead of the default, so
// a test with a fake environment can never reach the real desktop's players.
func ConnectAddress(address string) func(ctx context.Context) (Bus, error) {
	return func(ctx context.Context) (Bus, error) {
		conn, err := dbus.Connect(address, dbus.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("connect to session D-Bus: %w", err)
		}
		return &sessionBus{conn: conn}, nil
	}
}

func (b *sessionBus) Players(ctx context.Context) ([]Player, error) {
	var names []string
	if err := b.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return nil, fmt.Errorf("list session bus names: %w", err)
	}
	var players []Player
	for _, name := range names {
		if !strings.HasPrefix(name, busPrefix) {
			continue
		}
		var owner string
		if err := b.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, name).Store(&owner); err != nil {
			continue // the player quit between the two calls
		}
		players = append(players, Player{Name: name, Owner: owner})
	}
	sort.Slice(players, func(i, j int) bool { return players[i].Name < players[j].Name })
	return players, nil
}

func (b *sessionBus) Status(ctx context.Context, p Player) (string, error) {
	var value dbus.Variant
	call := b.conn.Object(p.Owner, objectPath).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, playerIF, "PlaybackStatus")
	if err := call.Store(&value); err != nil {
		return "", err
	}
	status, ok := value.Value().(string)
	if !ok {
		return "", fmt.Errorf("PlaybackStatus is %T, want a string", value.Value())
	}
	return status, nil
}

func (b *sessionBus) Pause(ctx context.Context, p Player) error {
	return b.conn.Object(p.Owner, objectPath).CallWithContext(ctx, playerIF+".Pause", 0).Err
}

func (b *sessionBus) Play(ctx context.Context, p Player) error {
	return b.conn.Object(p.Owner, objectPath).CallWithContext(ctx, playerIF+".Play", 0).Err
}

func (b *sessionBus) Close() error {
	return b.conn.Close()
}
