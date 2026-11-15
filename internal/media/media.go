// Package media pauses desktop media players for the duration of a recording
// and afterwards resumes exactly the players it paused (issue 125). It speaks
// MPRIS (org.mpris.MediaPlayer2) over the session D-Bus.
//
// Every pass runs in the background and fails open: a missing bus, a player
// that hangs or a failed call is logged and never delays or fails a recording.
package media

import (
	"context"
	"sync"
	"time"
)

// MPRIS playback states (the PlaybackStatus property).
const (
	StatusPlaying = "Playing"
	StatusPaused  = "Paused"
	StatusStopped = "Stopped"
)

// Player is one running MPRIS media player.
type Player struct {
	// Name is the well-known bus name, e.g. org.mpris.MediaPlayer2.firefox.instance_1_42.
	Name string
	// Owner is the unique connection name behind Name. Calls are addressed to
	// it, so a player that restarted during the recording is never resumed.
	Owner string
}

// Bus is the part of the session bus a Pauser needs. Its methods must be safe
// for concurrent use: one pass talks to all players at the same time.
type Bus interface {
	// Players lists the running MPRIS players.
	Players(ctx context.Context) ([]Player, error)
	// Status returns the player's PlaybackStatus.
	Status(ctx context.Context, p Player) (string, error)
	// Pause pauses playback.
	Pause(ctx context.Context, p Player) error
	// Play resumes playback.
	Play(ctx context.Context, p Player) error
	// Close releases the connection.
	Close() error
}

// Options configures a Pauser.
type Options struct {
	// Connect opens the bus for one pass. Nil selects the session D-Bus.
	Connect func(ctx context.Context) (Bus, error)
	// Timeout bounds one whole pause or resume pass (spec/eager.yaml media_pause.timeout_ms).
	Timeout time.Duration
	// Exclude lists bus names that are never paused.
	Exclude []string
	// Logf receives one line per paused or resumed player and per failure. Nil is silent.
	Logf func(format string, args ...any)
}

// Pauser pauses playing media players and later resumes the ones it paused.
// Passes are queued and run strictly in the order they were requested, so a
// Resume requested right after a Pause always sees what that Pause did.
// A nil *Pauser is valid and does nothing.
type Pauser struct {
	connect func(ctx context.Context) (Bus, error)
	timeout time.Duration
	exclude map[string]bool
	logf    func(format string, args ...any)

	mu   sync.Mutex
	tail chan struct{} // closed when the most recently queued pass has finished

	// paused holds the players this Pauser paused and has not resumed yet. Only
	// the running pass touches it; passes never overlap.
	paused []Player
}

// New returns a Pauser. It opens no connection until the first pass.
func New(opts Options) *Pauser {
	p := &Pauser{
		connect: opts.Connect,
		timeout: opts.Timeout,
		exclude: make(map[string]bool, len(opts.Exclude)),
		logf:    opts.Logf,
	}
	if p.connect == nil {
		p.connect = connectSessionBus
	}
	if p.logf == nil {
		p.logf = func(string, ...any) {}
	}
	for _, name := range opts.Exclude {
		p.exclude[name] = true
	}
	return p
}

// Pause queues a pass that pauses every player that is playing and remembers
// it. It returns at once; the channel closes when the pass has finished.
func (p *Pauser) Pause() <-chan struct{} {
	if p == nil {
		return closedChan()
	}
	return p.enqueue(p.pausePass)
}

// Resume queues a pass that resumes the players paused by earlier Pause passes,
// skipping any that are no longer paused (the user stopped or restarted them)
// or no longer running. It returns at once; the channel closes when the pass
// has finished.
func (p *Pauser) Resume() <-chan struct{} {
	if p == nil {
		return closedChan()
	}
	return p.enqueue(p.resumePass)
}

// Wait blocks until every pass queued so far has finished. Call it before the
// process exits, or a final Resume may never reach the players.
func (p *Pauser) Wait() {
	if p == nil {
		return
	}
	p.mu.Lock()
	tail := p.tail
	p.mu.Unlock()
	if tail != nil {
		<-tail
	}
}

func closedChan() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// enqueue runs pass after every earlier pass has finished.
func (p *Pauser) enqueue(pass func()) <-chan struct{} {
	p.mu.Lock()
	prev := p.tail
	done := make(chan struct{})
	p.tail = done
	p.mu.Unlock()
	go func() {
		defer close(done)
		if prev != nil {
			<-prev
		}
		pass()
	}()
	return done
}

func (p *Pauser) pausePass() {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	bus, err := p.connect(ctx)
	if err != nil {
		p.logf("media pause skipped: %v", err)
		return
	}
	defer bus.Close()
	players, err := bus.Players(ctx)
	if err != nil {
		p.logf("media pause skipped: %v", err)
		return
	}
	already := make(map[string]bool, len(p.paused))
	for _, pl := range p.paused {
		already[pl.Owner] = true
	}
	var candidates []Player
	for _, pl := range players {
		if !p.exclude[pl.Name] && !already[pl.Owner] {
			candidates = append(candidates, pl)
		}
	}
	paused := forEachPlayer(candidates, func(pl Player) bool {
		status, err := bus.Status(ctx, pl)
		if err != nil {
			p.logf("media pause: cannot read state of %s: %v", pl.Name, err)
			return false
		}
		if status != StatusPlaying {
			return false
		}
		if err := bus.Pause(ctx, pl); err != nil {
			p.logf("media pause: cannot pause %s: %v", pl.Name, err)
			return false
		}
		p.logf("paused media player %s for the recording", pl.Name)
		return true
	})
	p.paused = append(p.paused, paused...)
}

func (p *Pauser) resumePass() {
	players := p.paused
	p.paused = nil
	if len(players) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	bus, err := p.connect(ctx)
	if err != nil {
		p.logf("media resume skipped: %v", err)
		return
	}
	defer bus.Close()
	forEachPlayer(players, func(pl Player) bool {
		status, err := bus.Status(ctx, pl)
		if err != nil {
			p.logf("media resume: %s is gone or not answering, left alone: %v", pl.Name, err)
			return false
		}
		if status != StatusPaused {
			p.logf("media resume: %s is now %q, left alone", pl.Name, status)
			return false
		}
		if err := bus.Play(ctx, pl); err != nil {
			p.logf("media resume: cannot resume %s: %v", pl.Name, err)
			return false
		}
		p.logf("resumed media player %s", pl.Name)
		return true
	})
}

// forEachPlayer runs fn for all players at the same time, so one player that
// hangs cannot use up the budget of the others, and returns those for which fn
// reported true, in input order.
func forEachPlayer(players []Player, fn func(Player) bool) []Player {
	ok := make([]bool, len(players))
	var wg sync.WaitGroup
	for i, pl := range players {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok[i] = fn(pl)
		}()
	}
	wg.Wait()
	var out []Player
	for i, pl := range players {
		if ok[i] {
			out = append(out, pl)
		}
	}
	return out
}
