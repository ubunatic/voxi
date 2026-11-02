package monitor

import (
	"context"
	"math"
	"sync"
	"sync/atomic"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/eager"
	"ubunatic.com/voxi/spec"
)

// micCaptureGain is the digital gain the mic meter applies to each raw chunk
// (issue 214), so the meter shows the level voxi hears. It is the gain dictation
// applies to the default source: the spec's Bluetooth gain with the config
// override, and 1 for any other source. The capture goroutine reads it per
// chunk, so it is stored as float64 bits.
type micCaptureGain struct {
	bits atomic.Uint64
}

func newMicCaptureGain() *micCaptureGain {
	g := &micCaptureGain{}
	g.Set(1)
	return g
}

// Get returns the gain currently applied to chunks.
func (g *micCaptureGain) Get() float64 {
	return math.Float64frombits(g.bits.Load())
}

// Set replaces the gain applied to later chunks.
func (g *micCaptureGain) Set(v float64) {
	g.bits.Store(math.Float64bits(v))
}

// Filter applies the gain to one raw chunk through audio.ApplyGain, the same
// function dictation uses. At unity the chunk is returned as it is.
func (g *micCaptureGain) Filter(pcm []byte) []byte {
	v := g.Get()
	if v == 1 {
		return pcm
	}
	return audio.ApplyGain(pcm, v)
}

// loadedEagerSpec is the embedded spec/eager.yaml, parsed once. It panics only
// if the embedded spec is malformed, which spec's tests rule out.
var loadedEagerSpec = sync.OnceValue(func() *spec.EagerSpec {
	s, err := spec.LoadEager()
	if err != nil {
		panic(err)
	}
	return s
})

// refreshMicCaptureGain probes the default source and stores the gain dictation
// would apply to it. The monitor calls it on its collect cadence, so a change of
// default source is followed while the meter runs. A failed probe gives 1.
func refreshMicCaptureGain(ctx context.Context, d deps.Dependencies, g *micCaptureGain) {
	g.Set(eager.ProbeCaptureGain(ctx, d, loadedEagerSpec()))
}
