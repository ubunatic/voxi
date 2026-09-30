package mic

import (
	"sync"
	"time"
)

// SignalMonitor decides from the captured audio whether the mic is dead.
//
// Two signs, both measured over Window:
//   - stall: less than a quarter of the expected bytes arrived. Seen live when
//     a Bluetooth audio transport fails. Applies to every mic.
//   - zeros: every sample was exactly zero. A built-in mic always carries
//     noise, so this means muted or broken. Bluetooth headsets output exact
//     zeros while switching to headset mode and may gate out silence, so
//     zeros only count when ZerosMeanDead is set.
//
// Feed runs on the capture goroutine, Check on a timer; both are safe to call
// concurrently.
type SignalMonitor struct {
	Window        time.Duration
	BytesPerSec   int
	ZerosMeanDead bool

	mu        sync.Mutex
	start     time.Time
	lastSound time.Time
	arrivals  []arrival
}

type arrival struct {
	at time.Time
	n  int
}

// Start marks the capture start.
func (m *SignalMonitor) Start(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.start, m.lastSound, m.arrivals = now, now, nil
}

// SetZerosMeanDead switches zero detection on once the source is known.
func (m *SignalMonitor) SetZerosMeanDead(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ZerosMeanDead = v
}

// Feed records one captured frame.
func (m *SignalMonitor) Feed(frame []byte, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.arrivals = append(m.arrivals, arrival{now, len(frame)})
	for _, b := range frame {
		if b != 0 {
			m.lastSound = now
			break
		}
	}
	m.trim(now)
}

// Check reports whether the mic looks dead at now.
func (m *SignalMonitor) Check(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.start.IsZero() || now.Sub(m.start) < m.Window {
		return false
	}
	m.trim(now)
	got := 0
	for _, a := range m.arrivals {
		got += a.n
	}
	expected := int(m.Window.Seconds() * float64(m.BytesPerSec))
	if got*4 < expected {
		return true
	}
	return m.ZerosMeanDead && now.Sub(m.lastSound) >= m.Window
}

func (m *SignalMonitor) trim(now time.Time) {
	cut := 0
	for cut < len(m.arrivals) && now.Sub(m.arrivals[cut].at) > m.Window {
		cut++
	}
	m.arrivals = m.arrivals[cut:]
}
