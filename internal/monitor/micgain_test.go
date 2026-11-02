package monitor

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"

	"ubunatic.com/voxi/audiolevel"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/tts"
)

func gainPCM(samples ...int16) []byte {
	out := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[2*i:], uint16(s))
	}
	return out
}

// pwDumpNode is a minimal pw-dump whose default source is node. A bluez_ node
// is a Bluetooth input, anything else a built-in mic.
func pwDumpNode(node string) string {
	return `[
	{"type":"PipeWire:Interface:Metadata","id":0,"props":{"metadata.name":"default"},
	 "metadata":[{"key":"default.audio.source","value":{"name":"` + node + `"}}]},
	{"type":"PipeWire:Interface:Node","id":42,"info":{"props":{"media.class":"Audio/Source","node.name":"` + node + `"}}}
	]`
}

// fakeDefaultSource answers pw-dump from a mutable state, so a test can change
// the default source while the meter runs.
type fakeDefaultSource struct {
	mu  sync.Mutex
	out string
	err error
}

func (f *fakeDefaultSource) set(out string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.out, f.err = out, err
}

func (f *fakeDefaultSource) deps(home string) deps.Dependencies {
	return deps.Dependencies{
		Getenv: func(k string) string {
			if k == "HOME" {
				return home
			}
			return ""
		},
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		Run:      func(context.Context, string, ...string) error { return nil },
		RunOutput: func(_ context.Context, name string, _ ...string) (string, error) {
			if name != "pw-dump" {
				return "", errors.New("unexpected command " + name)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.out, f.err
		},
	}
}

func TestMicCaptureGainFilterMatchesDictationGain(t *testing.T) {
	g := newMicCaptureGain()
	g.Set(2.75)
	pcm := gainPCM(1000, -1000, 300)
	got := g.Filter(pcm)
	if want := audio.ApplyGain(pcm, 2.75); !bytes.Equal(got, want) {
		t.Fatalf("Filter() = %v, want the dictation gain %v", got, want)
	}
	if &got[0] == &pcm[0] {
		t.Fatal("Filter() must not alias the capture buffer")
	}
}

func TestMicCaptureGainUnityPassesThrough(t *testing.T) {
	g := newMicCaptureGain()
	pcm := gainPCM(1000, -1000)
	if got := g.Filter(pcm); &got[0] != &pcm[0] {
		t.Fatal("unity gain should return the chunk unchanged")
	}
}

func TestMicCaptureGainClipsAtFullScale(t *testing.T) {
	g := newMicCaptureGain()
	g.Set(2.75)
	got := g.Filter(gainPCM(20000, -20000))
	if s := int16(binary.LittleEndian.Uint16(got[0:])); s != math.MaxInt16 {
		t.Errorf("positive sample = %d, want %d", s, math.MaxInt16)
	}
	if s := int16(binary.LittleEndian.Uint16(got[2:])); s != math.MinInt16 {
		t.Errorf("negative sample = %d, want %d", s, math.MinInt16)
	}
}

// The meter moves up by the gain in dB: 20*log10(2.75) = 8.78 dB, measured on
// the meter's own dB scale.
func TestMicCaptureGainRaisesLevelByGainDB(t *testing.T) {
	g := newMicCaptureGain()
	g.Set(2.75)
	pcm := gainPCM(300, -300, 300, -300)
	raw := audiolevel.AmplitudeFromPCM16LE(pcm, audiolevel.DefaultMinDBFS)
	boosted := audiolevel.AmplitudeFromPCM16LE(g.Filter(pcm), audiolevel.DefaultMinDBFS)
	dBPerLevel := -audiolevel.DefaultMinDBFS / 100
	gotDB := (boosted - raw) * dBPerLevel
	wantDB := 20 * math.Log10(2.75)
	if math.Abs(gotDB-wantDB) > 0.05 {
		t.Fatalf("level rose %.2f dB, want %.2f dB", gotDB, wantDB)
	}
}

// A default source change while the meter runs moves the gain with it, and a
// failed probe falls back to unity (as dictation does).
func TestRefreshMicCaptureGainFollowsDefaultSource(t *testing.T) {
	src := &fakeDefaultSource{}
	d := src.deps(t.TempDir())
	g := newMicCaptureGain()

	src.set(pwDumpNode("bluez_input.AA"), nil)
	refreshMicCaptureGain(context.Background(), d, g)
	if got := g.Get(); got <= 1 {
		t.Fatalf("Bluetooth default: gain = %v, want the spec boost", got)
	}

	src.set(pwDumpNode("alsa_input.pci-0000_00_1f.3.analog-stereo"), nil)
	refreshMicCaptureGain(context.Background(), d, g)
	if got := g.Get(); got != 1 {
		t.Fatalf("built-in default: gain = %v, want 1", got)
	}

	src.set(pwDumpNode("bluez_input.AA"), nil)
	refreshMicCaptureGain(context.Background(), d, g)
	src.set("", errors.New("pw-dump failed"))
	refreshMicCaptureGain(context.Background(), d, g)
	if got := g.Get(); got != 1 {
		t.Fatalf("failed probe: gain = %v, want 1", got)
	}
}

// The default (full) view has its own status line with the mic glyph, so it
// carries the same gain label.
func TestStatusLineShowsGainOnFullView(t *testing.T) {
	render := func(gain float64) string {
		var out bytes.Buffer
		r := VoiceResourceReport{Mode: "eager", RecordStatus: "recording", MicLevel: 46, MicAvailable: true, MicGain: gain}
		PrintVoiceResourceReport(&out, r, ResourceSections{Speed: true})
		return out.String()
	}
	if !strings.Contains(render(2.75), "×2.75") {
		t.Errorf("gain 2.75: full view status line does not show ×2.75")
	}
	if strings.Contains(render(1), "×") {
		t.Errorf("gain 1: full view should not show a gain label")
	}
}

// The compact view names the boost so a visibly low-looking meter is explained.
func TestCompactMeterLabelShowsGain(t *testing.T) {
	base := VoiceResourceReport{RecordStatus: "recording", MicLevel: 46}
	render := func(r VoiceResourceReport) string {
		var out bytes.Buffer
		PrintCompactTwoBox(&out, r, []chunks.Chunk{}, tts.Snapshot{}, 100)
		return out.String()
	}

	boosted := base
	boosted.MicGain = 2.75
	if !strings.Contains(render(boosted), "×2.75") {
		t.Errorf("gain 2.75: compact view does not show ×2.75")
	}
	if strings.Contains(render(base), "×") {
		t.Errorf("unset gain: compact view should not show a gain label")
	}
	unity := base
	unity.MicGain = 1
	if strings.Contains(render(unity), "×") {
		t.Errorf("gain 1: compact view should not show a gain label")
	}
}
