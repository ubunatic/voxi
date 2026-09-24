package tts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	maxTextBytes       = 1 << 20
	maxQueuedChunks    = 512
	statusIdle         = "idle"
	statusSynthesizing = "synthesizing"
	statusPlaying      = "playing"
	statusPaused       = "paused"
)

// ErrTTSDisabled indicates TTS is turned off in the user's configuration.
var ErrTTSDisabled = errors.New("TTS is disabled by configuration (set tts_enabled: true to enable it)")

// Snapshot is the monitor panel's current playback and queue view.
type Snapshot struct {
	Status           string
	BackendStatus    string
	Current          string
	Queue            []string
	TimeToFirstAudio time.Duration
	LastError        string
	History          []ChunkRecord
}

// ChunkRecord is one TTS chunk's single monitor-feed row, updated through its lifecycle.
type ChunkRecord struct {
	ID           uint64
	Text, Status string
	Timestamp    time.Time
	UpdatedAt    time.Time
}

type command struct {
	action Action
	text   string
	reply  chan commandResult
}

type commandResult struct {
	count int
	err   error
}

type synthResult struct {
	index    int
	audio    audioFile
	duration time.Duration
	err      error
}

type synthJob struct {
	index  int
	cancel context.CancelFunc
	done   <-chan synthResult
}

// Manager owns the in-memory queue for one monitor watch session.
type Manager struct {
	ctx              context.Context
	cancel           context.CancelFunc
	backend          EngineBackend
	enabled          bool
	backendStatus    string
	commands         chan command
	done             chan struct{}
	mu               sync.RWMutex
	snapshot         Snapshot
	feedHistory      []ChunkRecord
	nextFeedID       uint64
	feedCurrentID    uint64
	feedCurrent      string
	feedCurrentIndex int
}

// NewManager starts a monitor-scoped queue and playback worker.
func NewManager(ctx context.Context, backend EngineBackend) *Manager {
	return NewManagerWithEnabled(ctx, backend, true)
}

// NewManagerWithEnabled creates a monitor-scoped manager respecting the user's TTS setting.
func NewManagerWithEnabled(ctx context.Context, backend EngineBackend, enabled bool) *Manager {
	managerCtx, cancel := context.WithCancel(ctx)
	backendStatus := "unknown"
	if status, ok := backend.(interface{ BackendStatus() string }); ok {
		backendStatus = status.BackendStatus()
	}
	if !enabled {
		backendStatus = "disabled by configuration"
	}
	m := &Manager{
		ctx:           managerCtx,
		cancel:        cancel,
		backend:       backend,
		enabled:       enabled,
		backendStatus: backendStatus,
		commands:      make(chan command, 32),
		done:          make(chan struct{}),
		snapshot:      Snapshot{Status: statusIdle, BackendStatus: backendStatus},
	}
	go m.run()
	return m
}

// Enqueue appends text chunks to the monitor's queue.
func (m *Manager) Enqueue(text string) (int, error) {
	if !m.enabled {
		return 0, ErrTTSDisabled
	}
	if len(text) > maxTextBytes {
		return 0, fmt.Errorf("text exceeds %d byte limit", maxTextBytes)
	}
	chunks := SplitText(text)
	if len(chunks) == 0 {
		return 0, errors.New("text is empty")
	}
	res, err := m.request(command{action: "say", text: text})
	return res.count, err
}

// Control applies a monitor playback control.
func (m *Manager) Control(action Action) error {
	if !m.enabled {
		return ErrTTSDisabled
	}
	_, err := m.request(command{action: action})
	return err
}

// Snapshot returns a copy of the current monitor display state.
func (m *Manager) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.snapshot
	s.Queue = append([]string(nil), s.Queue...)
	s.History = append([]ChunkRecord(nil), s.History...)
	return s
}

// Close stops playback and discards this monitor session's queue.
func (m *Manager) Close() {
	m.cancel()
	<-m.done
}

func (m *Manager) request(c command) (commandResult, error) {
	c.reply = make(chan commandResult, 1)
	select {
	case m.commands <- c:
	case <-m.ctx.Done():
		return commandResult{}, ErrNoMonitor
	}
	select {
	case res := <-c.reply:
		return res, res.err
	case <-m.ctx.Done():
		return commandResult{}, ErrNoMonitor
	}
}

func (m *Manager) publish(status, current string, items []string, cursor int, first time.Duration, lastErr string) {
	m.recordFeed(status, current, cursor)
	queue := make([]string, 0, len(items))
	start := cursor
	if current != "" {
		start++
	}
	for i := start; i < len(items); i++ {
		queue = append(queue, items[i])
	}
	m.mu.Lock()
	m.snapshot = Snapshot{Status: status, BackendStatus: m.backendStatus, Current: current, Queue: queue, TimeToFirstAudio: first, LastError: lastErr, History: append([]ChunkRecord(nil), m.feedHistory...)}
	m.mu.Unlock()
}

func (m *Manager) recordFeed(status, current string, index int) {
	if current == "" {
		return
	}
	if index != m.feedCurrentIndex || current != m.feedCurrent || m.feedCurrentID == 0 {
		if m.feedCurrentID != 0 {
			for i := range m.feedHistory {
				if m.feedHistory[i].ID == m.feedCurrentID {
					m.feedHistory[i].Status = "played"
					m.feedHistory[i].UpdatedAt = time.Now()
				}
			}
		}
		m.nextFeedID++
		m.feedCurrentID = m.nextFeedID
		m.feedCurrent = current
		m.feedCurrentIndex = index
		created := time.Now()
		m.feedHistory = retainFeedRow(m.feedHistory, ChunkRecord{ID: m.feedCurrentID, Text: current, Status: status, Timestamp: created, UpdatedAt: created})
	}
	for i := range m.feedHistory {
		if m.feedHistory[i].ID == m.feedCurrentID && m.feedHistory[i].Status != status {
			m.feedHistory[i].Status = status
			m.feedHistory[i].UpdatedAt = time.Now()
		}
	}
}

func retainFeedRow(history []ChunkRecord, row ChunkRecord) []ChunkRecord {
	history = append(history, row)
	if len(history) > 10 {
		history = append([]ChunkRecord(nil), history[len(history)-10:]...)
	}
	return history
}

func (m *Manager) finishFeed(status string) {
	if m.feedCurrentID == 0 {
		return
	}
	for i := range m.feedHistory {
		if m.feedHistory[i].ID == m.feedCurrentID {
			m.feedHistory[i].Status = status
			m.feedHistory[i].UpdatedAt = time.Now()
		}
	}
	m.feedCurrentID = 0
	m.feedCurrent = ""
}

func (m *Manager) run() {
	defer close(m.done)
	items := make([]string, 0)
	cursor := 0
	current := ""
	status := statusIdle
	paused := false
	firstQueuedAt := time.Time{}
	firstAudio := time.Duration(0)
	lastErr := ""
	var currentAudio audioFile
	var player Playback
	var currentJob *synthJob
	var prefetchJob *synthJob
	var prefetched *synthResult

	cleanupCurrent := func() {
		if player != nil {
			_ = player.Stop()
			player = nil
		}
		if currentAudio.path != "" {
			_ = currentAudio.Close()
			currentAudio = audioFile{}
		}
		if currentJob != nil {
			job := currentJob
			job.cancel()
			result := <-job.done
			_ = result.audio.Close()
			currentJob = nil
		}
	}
	cleanupPrefetch := func() {
		if prefetchJob != nil {
			job := prefetchJob
			job.cancel()
			result := <-job.done
			_ = result.audio.Close()
			prefetchJob = nil
		}
		if prefetched != nil {
			_ = prefetched.audio.Close()
			prefetched = nil
		}
	}
	startSynthesis := func(index int) *synthJob {
		jobCtx, cancel := context.WithCancel(m.ctx)
		result := make(chan synthResult, 1)
		text := items[index]
		go func() {
			audio, elapsed, err := m.backend.Synthesize(jobCtx, text)
			if jobCtx.Err() != nil {
				_ = audio.Close()
				if err == nil {
					err = jobCtx.Err()
				}
			}
			result <- synthResult{index: index, audio: audio, duration: elapsed, err: err}
		}()
		return &synthJob{index: index, cancel: cancel, done: result}
	}
	startPlayer := func(result synthResult) {
		currentAudio = result.audio
		var err error
		player, err = m.backend.StartPlayback(m.ctx, currentAudio.path)
		if err != nil {
			lastErr = err.Error()
			_ = currentAudio.Close()
			currentAudio = audioFile{}
			current = ""
			cursor++
			pruneQueueHistory(&items, &cursor)
			status = statusIdle
			return
		}
		if paused {
			_ = player.Pause()
			status = statusPaused
		} else {
			status = statusPlaying
		}
		if firstAudio == 0 && !firstQueuedAt.IsZero() {
			firstAudio = time.Since(firstQueuedAt)
		}
	}
	for {
		if m.ctx.Err() != nil {
			m.finishFeed("stopped")
			cleanupCurrent()
			cleanupPrefetch()
			if currentJob != nil {
				<-currentJob.done
			}
			if prefetchJob != nil {
				<-prefetchJob.done
			}
			m.publish(statusIdle, "", nil, 0, firstAudio, lastErr)
			return
		}
		if player == nil && currentJob == nil && cursor < len(items) {
			current = items[cursor]
			if prefetched != nil && prefetched.index == cursor {
				ready := *prefetched
				prefetched = nil
				startPlayer(ready)
			} else if prefetchJob != nil && prefetchJob.index == cursor {
				currentJob = prefetchJob
				prefetchJob = nil
				status = statusSynthesizing
			} else {
				currentJob = startSynthesis(cursor)
				status = statusSynthesizing
			}
		}
		if player != nil && prefetchJob == nil && prefetched == nil && cursor+1 < len(items) {
			prefetchJob = startSynthesis(cursor + 1)
		}
		m.publish(status, current, items, cursor, firstAudio, lastErr)

		var synthDone <-chan synthResult
		if currentJob != nil {
			synthDone = currentJob.done
		}
		var prefetchDone <-chan synthResult
		if prefetchJob != nil {
			prefetchDone = prefetchJob.done
		}
		var playerDone <-chan error
		if player != nil {
			playerDone = player.Done()
		}
		select {
		case <-m.ctx.Done():
			continue
		case c := <-m.commands:
			if c.action == ActionNext || (c.action == ActionPrevious && cursor > 0) {
				m.finishFeed("skipped")
			}
			if c.action == ActionStop {
				m.finishFeed("stopped")
			}
			res := m.applyCommand(c, &items, &cursor, &current, &status, &paused, &firstQueuedAt, &firstAudio,
				&currentAudio, &player, &currentJob, &prefetchJob, &prefetched, &lastErr, cleanupCurrent, cleanupPrefetch)
			if res.err != nil {
				lastErr = res.err.Error()
			}
			c.reply <- res
		case result := <-synthDone:
			if currentJob == nil || result.index != currentJob.index || result.index != cursor {
				_ = result.audio.Close()
				continue
			}
			currentJob.cancel()
			currentJob = nil
			if result.err != nil {
				if !errors.Is(result.err, context.Canceled) {
					lastErr = result.err.Error()
					m.finishFeed("stopped")
					cursor++
					pruneQueueHistory(&items, &cursor)
				}
				current = ""
				status = statusIdle
				continue
			}
			startPlayer(result)
		case result := <-prefetchDone:
			if prefetchJob == nil || result.index != prefetchJob.index || result.index != cursor+1 {
				_ = result.audio.Close()
				continue
			}
			prefetchJob.cancel()
			prefetchJob = nil
			if result.err != nil {
				if !errors.Is(result.err, context.Canceled) {
					lastErr = result.err.Error()
				}
			} else {
				prefetched = &result
			}
		case err := <-playerDone:
			if err == nil {
				m.finishFeed("played")
			} else {
				m.finishFeed("stopped")
			}
			if err != nil {
				lastErr = err.Error()
			}
			if currentAudio.path != "" {
				_ = currentAudio.Close()
				currentAudio = audioFile{}
			}
			player = nil
			current = ""
			status = statusIdle
			cursor++
			pruneQueueHistory(&items, &cursor)
		}
	}
}

func (m *Manager) applyCommand(
	c command,
	items *[]string,
	cursor *int,
	current *string,
	status *string,
	paused *bool,
	firstQueuedAt *time.Time,
	firstAudio *time.Duration,
	currentAudio *audioFile,
	player *Playback,
	currentJob **synthJob,
	prefetchJob **synthJob,
	prefetched **synthResult,
	lastErr *string,
	cleanupCurrent func(),
	cleanupPrefetch func(),
) commandResult {
	switch c.action {
	case "say":
		newChunks := SplitText(c.text)
		if len(newChunks) == 0 {
			return commandResult{err: errors.New("text is empty")}
		}
		if len(*items)+len(newChunks)-*cursor > maxQueuedChunks {
			return commandResult{err: fmt.Errorf("queue exceeds %d chunks", maxQueuedChunks)}
		}
		if *player == nil && *currentJob == nil && *cursor >= len(*items) {
			*cursor = len(*items)
			*firstQueuedAt = time.Now()
			*firstAudio = 0
		}
		*items = append(*items, newChunks...)
		*lastErr = ""
		return commandResult{count: len(newChunks)}
	case ActionPlayPause:
		*paused = !*paused
		if *player != nil {
			var err error
			if *paused {
				err = (*player).Pause()
				*status = statusPaused
			} else {
				err = (*player).Resume()
				*status = statusPlaying
			}
			return commandResult{err: err}
		}
		return commandResult{}
	case ActionPause:
		if *paused {
			return commandResult{}
		}
		*paused = true
		if *player != nil {
			err := (*player).Pause()
			*status = statusPaused
			return commandResult{err: err}
		}
		if *currentJob != nil || *cursor < len(*items) {
			*status = statusPaused
		}
		return commandResult{}
	case ActionResume:
		if !*paused {
			return commandResult{}
		}
		*paused = false
		if *player != nil {
			err := (*player).Resume()
			*status = statusPlaying
			return commandResult{err: err}
		}
		if *currentJob != nil || *cursor < len(*items) {
			*status = statusSynthesizing
		} else {
			*status = statusIdle
		}
		return commandResult{}
	case ActionPrevious:
		if *cursor == 0 {
			return commandResult{}
		}
		cleanupCurrent()
		cleanupPrefetch()
		*currentAudio = audioFile{}
		*player = nil
		*currentJob = nil
		*cursor--
		*current = ""
		*status = statusIdle
		*paused = false
		return commandResult{}
	case ActionNext:
		cleanupCurrent()
		cleanupPrefetch()
		*currentAudio = audioFile{}
		*player = nil
		*currentJob = nil
		if *cursor < len(*items) {
			*cursor++
			pruneQueueHistory(items, cursor)
		}
		*current = ""
		*status = statusIdle
		*paused = false
		return commandResult{}
	case ActionStop:
		cleanupCurrent()
		cleanupPrefetch()
		*items = nil
		*cursor = 0
		*current = ""
		*status = statusIdle
		*paused = false
		*firstQueuedAt = time.Time{}
		*firstAudio = 0
		return commandResult{}
	case ActionClear:
		cleanupPrefetch()
		end := *cursor
		if *player != nil || *currentJob != nil {
			end++
		}
		if end < len(*items) {
			*items = (*items)[:end]
		}
		if *cursor > len(*items) {
			*cursor = len(*items)
		}
		return commandResult{}
	default:
		return commandResult{err: fmt.Errorf("unknown playback control %q", strings.TrimSpace(string(c.action)))}
	}
}

func pruneQueueHistory(items *[]string, cursor *int) {
	if *cursor <= 1 {
		return
	}
	*items = (*items)[*cursor-1:]
	*cursor = 1
}
