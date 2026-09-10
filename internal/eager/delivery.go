package eager

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/telemetry"
	"ubunatic.com/voxi/internal/typing"
)

// deliveryLedger is the durable at-most-once fence for accepted eager
// transcripts. A claim is persisted before the injector is called. Therefore
// a crash can lose text after a claim, but can never cause a claimed identity
// to be submitted a second time after restart.
type deliveryLedger struct {
	path string
	mu   sync.Mutex
	seen map[string]struct{}
}

type deliveryClaim struct {
	Identity string    `json:"identity"`
	Claimed  time.Time `json:"claimed_at"`
}

type injectorObserver struct {
	recorder   *telemetry.Recorder
	sessionID  string
	chunkID    string
	chunkIndex int
	deliveryID string
}

func (o *injectorObserver) Started(path string, at time.Time) {
	_ = o.recorder.Record(telemetry.Event{Event: telemetry.InjectorStarted, Timestamp: at, SessionID: o.sessionID, ChunkID: o.chunkID, ChunkIndex: o.chunkIndex, DeliveryID: o.deliveryID, InjectorPath: path, Attempt: 1})
}

func (o *injectorObserver) Completed(a typing.InjectionAttempt) {
	duration := a.EndedAt.Sub(a.StartedAt).Seconds() * 1000
	success := a.Err == nil
	e := telemetry.Event{Event: telemetry.InjectorComplete, Timestamp: a.EndedAt, SessionID: o.sessionID, ChunkID: o.chunkID, ChunkIndex: o.chunkIndex, DeliveryID: o.deliveryID, InjectorPath: a.Path, ProcessID: a.PID, Attempt: 1, Success: &success, DurationMS: &duration}
	if a.Err != nil {
		e.Error = a.Err.Error()
		if a.Err == context.Canceled {
			e.CancelReason = "context_canceled"
		}
	}
	_ = o.recorder.Record(e)
}

func newDeliveryLedger(path string) *deliveryLedger {
	return &deliveryLedger{path: path, seen: make(map[string]struct{})}
}

// Claim atomically records identity and returns false for a prior claim. The
// advisory lock also protects overlapping eager sessions and a concurrently
// restarted daemon sharing the same ledger.
func (l *deliveryLedger) Claim(identity string) (bool, error) {
	if identity == "" {
		return false, fmt.Errorf("empty delivery identity")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.seen[identity]; ok {
		return false, nil
	}
	if l.path == "" {
		l.seen[identity] = struct{}{}
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
		return false, fmt.Errorf("create delivery ledger directory: %w", err)
	}
	lock, err := os.OpenFile(l.path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return false, fmt.Errorf("open delivery ledger lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return false, fmt.Errorf("lock delivery ledger: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	// Reload while locked so a process that started before a daemon restart
	// cannot rely solely on its stale in-memory cache.
	f, err := os.Open(l.path)
	if err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var row deliveryClaim
			if json.Unmarshal(scanner.Bytes(), &row) == nil && row.Identity != "" {
				l.seen[row.Identity] = struct{}{}
			}
		}
		_ = f.Close()
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read delivery ledger: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("open delivery ledger: %w", err)
	}
	if _, ok := l.seen[identity]; ok {
		return false, nil
	}
	f, err = os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return false, fmt.Errorf("open delivery ledger: %w", err)
	}
	defer f.Close()
	row, err := json.Marshal(deliveryClaim{Identity: identity, Claimed: time.Now().UTC()})
	if err != nil {
		return false, fmt.Errorf("encode delivery claim: %w", err)
	}
	if _, err := f.Write(append(row, '\n')); err != nil {
		return false, fmt.Errorf("persist delivery claim: %w", err)
	}
	if err := f.Sync(); err != nil {
		return false, fmt.Errorf("sync delivery claim: %w", err)
	}
	_ = f.Chmod(0600)
	l.seen[identity] = struct{}{}
	return true, nil
}
