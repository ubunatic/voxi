package media

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBus is an in-memory set of MPRIS players. It is shared by every pass of
// one test, like the real session bus, and records the calls it received.
type fakeBus struct {
	mu         sync.Mutex
	order      []string          // player names, in listing order
	owners     map[string]string // name -> unique owner
	status     map[string]string // owner -> PlaybackStatus
	calls      []string          // "Pause <name>", "Play <name>", "Status <name>"
	connects   int
	connectErr error
	listErr    error
	pauseErr   map[string]error // by name
	gone       map[string]bool  // by name: every call fails
	// hooks run outside the lock, before the call takes effect.
	beforeList  func()
	beforePause func(ctx context.Context, name string)
}

func newFakeBus(players ...string) *fakeBus {
	b := &fakeBus{owners: map[string]string{}, status: map[string]string{}, pauseErr: map[string]error{}, gone: map[string]bool{}}
	for i, spec := range players {
		name, status, _ := strings.Cut(spec, "=")
		owner := fmt.Sprintf(":1.%d", i+1)
		b.order = append(b.order, name)
		b.owners[name] = owner
		b.status[owner] = status
	}
	return b
}

func (b *fakeBus) connect(context.Context) (Bus, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connects++
	if b.connectErr != nil {
		return nil, b.connectErr
	}
	return b, nil
}

func (b *fakeBus) Players(context.Context) ([]Player, error) {
	if b.beforeList != nil {
		b.beforeList()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.listErr != nil {
		return nil, b.listErr
	}
	var out []Player
	for _, name := range b.order {
		out = append(out, Player{Name: name, Owner: b.owners[name]})
	}
	return out, nil
}

func (b *fakeBus) Status(_ context.Context, p Player) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, "Status "+p.Name)
	if b.gone[p.Name] {
		return "", errors.New("name has no owner")
	}
	return b.status[p.Owner], nil
}

func (b *fakeBus) Pause(ctx context.Context, p Player) error {
	if b.beforePause != nil {
		b.beforePause(ctx, p.Name)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, "Pause "+p.Name)
	if err := b.pauseErr[p.Name]; err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.status[p.Owner] = StatusPaused
	return nil
}

func (b *fakeBus) Play(_ context.Context, p Player) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, "Play "+p.Name)
	if b.gone[p.Name] {
		return errors.New("name has no owner")
	}
	b.status[p.Owner] = StatusPlaying
	return nil
}

func (b *fakeBus) Close() error { return nil }

// set changes a player's state the way the user or the player itself would.
func (b *fakeBus) set(name, status string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status[b.owners[name]] = status
}

func (b *fakeBus) statusOf(name string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status[b.owners[name]]
}

// did returns the recorded calls of one verb, sorted (a pass talks to all
// players at the same time, so their order is not defined).
func (b *fakeBus) did(verb string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, call := range b.calls {
		if strings.HasPrefix(call, verb+" ") {
			out = append(out, strings.TrimPrefix(call, verb+" "))
		}
	}
	sort.Strings(out)
	return out
}

type logSink struct {
	mu    sync.Mutex
	lines []string
}

func (l *logSink) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *logSink) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func newTestPauser(bus *fakeBus, log *logSink, exclude ...string) *Pauser {
	opts := Options{Connect: bus.connect, Timeout: time.Minute, Exclude: exclude}
	if log != nil {
		opts.Logf = log.logf
	}
	return New(opts)
}

func wantCalls(t *testing.T, bus *fakeBus, verb string, want ...string) {
	t.Helper()
	got := bus.did(verb)
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s calls = %v, want %v", verb, got, want)
	}
}

func TestPauseThenResume(t *testing.T) {
	const (
		music   = "org.mpris.MediaPlayer2.music"
		browser = "org.mpris.MediaPlayer2.browser"
		video   = "org.mpris.MediaPlayer2.video"
		idle    = "org.mpris.MediaPlayer2.idle"
	)
	cases := []struct {
		name string
		// during changes player states while the recording runs.
		during    func(bus *fakeBus)
		wantPause []string
		wantPlay  []string
		wantFinal map[string]string
	}{
		{
			name:      "resumes only the players it paused",
			wantPause: []string{browser, music},
			wantPlay:  []string{browser, music},
			wantFinal: map[string]string{music: StatusPlaying, browser: StatusPlaying, video: StatusPaused, idle: StatusStopped},
		},
		{
			name:      "leaves a player the user stopped during the recording",
			during:    func(bus *fakeBus) { bus.set(music, StatusStopped) },
			wantPause: []string{browser, music},
			wantPlay:  []string{browser},
			wantFinal: map[string]string{music: StatusStopped, browser: StatusPlaying},
		},
		{
			name:      "does not touch a player the user already resumed",
			during:    func(bus *fakeBus) { bus.set(browser, StatusPlaying) },
			wantPause: []string{browser, music},
			wantPlay:  []string{music},
			wantFinal: map[string]string{music: StatusPlaying, browser: StatusPlaying},
		},
		{
			name: "leaves a player that quit during the recording",
			during: func(bus *fakeBus) {
				bus.mu.Lock()
				bus.gone[music] = true
				bus.mu.Unlock()
			},
			wantPause: []string{browser, music},
			wantPlay:  []string{browser},
		},
		{
			name:      "never resumes a player the user paused during the recording",
			during:    func(bus *fakeBus) { bus.set(video, StatusPaused) },
			wantPause: []string{browser, music},
			wantPlay:  []string{browser, music},
			wantFinal: map[string]string{video: StatusPaused},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bus := newFakeBus(music+"=Playing", browser+"=Playing", video+"=Paused", idle+"=Stopped")
			p := newTestPauser(bus, nil)
			<-p.Pause()
			wantCalls(t, bus, "Pause", c.wantPause...)
			if c.during != nil {
				c.during(bus)
			}
			<-p.Resume()
			wantCalls(t, bus, "Play", c.wantPlay...)
			for name, want := range c.wantFinal {
				if got := bus.statusOf(name); got != want {
					t.Errorf("%s is %q after the recording, want %q", name, got, want)
				}
			}
			// A second Resume has nothing left to do.
			connects := bus.connects
			<-p.Resume()
			wantCalls(t, bus, "Play", c.wantPlay...)
			if bus.connects != connects {
				t.Errorf("Resume with nothing paused opened the bus")
			}
		})
	}
}

func TestExcludedPlayersAreNeverTouched(t *testing.T) {
	const own = "org.mpris.MediaPlayer2.voxi"
	const music = "org.mpris.MediaPlayer2.music"
	bus := newFakeBus(own+"=Playing", music+"=Playing")
	p := newTestPauser(bus, nil, own)
	<-p.Pause()
	<-p.Resume()
	wantCalls(t, bus, "Pause", music)
	wantCalls(t, bus, "Play", music)
	for _, call := range bus.did("Status") {
		if call == own {
			t.Errorf("read the state of excluded player %s", own)
		}
	}
	if got := bus.statusOf(own); got != StatusPlaying {
		t.Errorf("excluded player is %q, want it untouched", got)
	}
}

func TestFailuresAreLoggedAndNeverFatal(t *testing.T) {
	const music = "org.mpris.MediaPlayer2.music"
	const browser = "org.mpris.MediaPlayer2.browser"
	t.Run("no session bus", func(t *testing.T) {
		bus := newFakeBus(music + "=Playing")
		bus.connectErr = errors.New("no bus")
		log := &logSink{}
		p := newTestPauser(bus, log)
		<-p.Pause()
		<-p.Resume()
		wantCalls(t, bus, "Pause")
		wantCalls(t, bus, "Play")
		if !strings.Contains(log.text(), "no bus") {
			t.Errorf("log = %q, want the connect error", log.text())
		}
	})
	t.Run("listing fails", func(t *testing.T) {
		bus := newFakeBus(music + "=Playing")
		bus.listErr = errors.New("list failed")
		log := &logSink{}
		p := newTestPauser(bus, log)
		<-p.Pause()
		<-p.Resume()
		wantCalls(t, bus, "Play")
		if !strings.Contains(log.text(), "list failed") {
			t.Errorf("log = %q, want the listing error", log.text())
		}
	})
	t.Run("a player that refuses Pause is not resumed", func(t *testing.T) {
		bus := newFakeBus(music+"=Playing", browser+"=Playing")
		bus.pauseErr[music] = errors.New("not supported")
		log := &logSink{}
		p := newTestPauser(bus, log)
		<-p.Pause()
		// The user pauses it by hand; voxi must not undo that.
		bus.set(music, StatusPaused)
		<-p.Resume()
		wantCalls(t, bus, "Play", browser)
		if !strings.Contains(log.text(), "cannot pause "+music) {
			t.Errorf("log = %q, want the refused pause", log.text())
		}
	})
	t.Run("nil pauser", func(t *testing.T) {
		var p *Pauser
		<-p.Pause()
		<-p.Resume()
		p.Wait()
	})
}

// A Resume requested while the Pause pass is still running must wait for it:
// otherwise it would find nothing to resume and playback would stay paused.
func TestResumeWaitsForRunningPause(t *testing.T) {
	const music = "org.mpris.MediaPlayer2.music"
	bus := newFakeBus(music + "=Playing")
	release := make(chan struct{})
	listing := make(chan struct{})
	var once sync.Once
	bus.beforeList = func() {
		once.Do(func() {
			close(listing)
			<-release
		})
	}
	p := newTestPauser(bus, nil)
	p.Pause()
	<-listing // the pause pass is now running and blocked
	resumed := p.Resume()
	select {
	case <-resumed:
		t.Fatal("Resume finished before the Pause pass it follows")
	default:
	}
	close(release)
	p.Wait()
	wantCalls(t, bus, "Pause", music)
	wantCalls(t, bus, "Play", music)
	if got := bus.statusOf(music); got != StatusPlaying {
		t.Errorf("player is %q after pause and resume, want Playing", got)
	}
}

// Two recordings in a row: the second Pause must run after the first Resume,
// and a repeated Pause must not make a player count twice.
func TestPassesKeepTheirOrder(t *testing.T) {
	const music = "org.mpris.MediaPlayer2.music"
	bus := newFakeBus(music + "=Playing")
	p := newTestPauser(bus, nil)
	p.Pause()
	p.Pause()
	p.Resume()
	p.Pause()
	p.Resume()
	p.Wait()
	bus.mu.Lock()
	var got []string
	for _, call := range bus.calls {
		if !strings.HasPrefix(call, "Status ") {
			got = append(got, call)
		}
	}
	bus.mu.Unlock()
	want := []string{"Pause " + music, "Play " + music, "Pause " + music, "Play " + music}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

// One player that never answers must not stop the others from being paused.
func TestHungPlayerDoesNotBlockOthers(t *testing.T) {
	const hung = "org.mpris.MediaPlayer2.hung"
	const music = "org.mpris.MediaPlayer2.music"
	bus := newFakeBus(hung+"=Playing", music+"=Playing")
	release := make(chan struct{})
	musicPaused := make(chan struct{})
	bus.beforePause = func(_ context.Context, name string) {
		if name == hung {
			<-musicPaused // proves music was paused while this call was still open
			<-release
		}
	}
	p := newTestPauser(bus, nil)
	done := p.Pause()
	go func() {
		for bus.statusOf(music) != StatusPaused {
			select {
			case <-done:
				return
			default:
			}
		}
		close(musicPaused)
	}()
	<-musicPaused
	select {
	case <-done:
		t.Fatal("pause pass finished while one player was still hanging")
	default:
	}
	close(release)
	<-done
	wantCalls(t, bus, "Pause", hung, music)
}
