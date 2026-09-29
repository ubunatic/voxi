package sample

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
)

const (
	// DefaultMergeGap is the silence inserted between merged parts.
	DefaultMergeGap = 300 * time.Millisecond
	// DefaultMergeMax matches VoxCPM's reference encoder capacity
	// (spec/tts.yaml: audiovae_encoder_sample_capacity=320000 at 16 kHz).
	DefaultMergeMax = 20 * time.Second
	// maxChunkGap is the largest recorded pause between adjacent chunks
	// that still counts as one coherent utterance without --force.
	maxChunkGap = 30 * time.Second
	// loudnessRatio is the RMS ratio between the loudest and quietest part
	// above which merge warns.
	loudnessRatio = 2.0
)

// mergePart is one input of a merge: a sample or a chunk.
type mergePart struct {
	label      string
	wav        string
	transcript string
	keyterms   []string
}

// wavPCM is the decoded content of a 16-bit PCM WAV file.
type wavPCM struct {
	rate, channels, bits int
	pcm                  []byte
}

func readWAV(path string) (wavPCM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return wavPCM{}, err
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return wavPCM{}, fmt.Errorf("%s: not a RIFF/WAVE file", path)
	}
	var w wavPCM
	haveData := false
	for pos := 12; pos+8 <= len(data); {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(data) {
			return wavPCM{}, fmt.Errorf("%s: truncated %q chunk", path, id)
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return wavPCM{}, fmt.Errorf("%s: truncated fmt chunk", path)
			}
			if format := binary.LittleEndian.Uint16(data[body : body+2]); format != 1 {
				return wavPCM{}, fmt.Errorf("%s: WAV format %d is not PCM", path, format)
			}
			w.channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
			w.rate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			w.bits = int(binary.LittleEndian.Uint16(data[body+14 : body+16]))
		case "data":
			w.pcm = data[body : body+size]
			haveData = true
		}
		pos = body + size + size%2
	}
	if w.rate == 0 || !haveData {
		return wavPCM{}, fmt.Errorf("%s: missing fmt or data chunk", path)
	}
	return w, nil
}

// joinAudio concatenates the parts' PCM with gap silence in between. Every
// part must be 16-bit mono PCM at the same rate; nothing is resampled.
func joinAudio(parts []mergePart, gap, maxDur time.Duration) (pcm []byte, rate int, warnings []string, err error) {
	minRMS, maxRMS := -1, 0
	var minLabel, maxLabel string
	for i, p := range parts {
		w, err := readWAV(p.wav)
		if err != nil {
			return nil, 0, nil, fmt.Errorf("%s: %w", p.label, err)
		}
		if w.channels != 1 || w.bits != 16 {
			return nil, 0, nil, fmt.Errorf("%s: %d channel(s), %d bit; merge needs 16-bit mono PCM", p.label, w.channels, w.bits)
		}
		if len(w.pcm)%2 != 0 {
			return nil, 0, nil, fmt.Errorf("%s: data chunk ends in a partial 16-bit sample", p.label)
		}
		if i == 0 {
			rate = w.rate
		} else if w.rate != rate {
			return nil, 0, nil, fmt.Errorf("%s: sample rate %d Hz differs from %d Hz of %s", p.label, w.rate, rate, parts[0].label)
		}
		if i > 0 {
			pcm = append(pcm, make([]byte, 2*int(gap.Seconds()*float64(rate)))...)
		}
		pcm = append(pcm, w.pcm...)
		rms := audio.ComputeAudioRMS(w.pcm)
		if minRMS < 0 || rms < minRMS {
			minRMS, minLabel = rms, p.label
		}
		if rms > maxRMS {
			maxRMS, maxLabel = rms, p.label
		}
	}
	if dur := time.Duration(float64(len(pcm)) / float64(2*rate) * float64(time.Second)); dur > maxDur {
		return nil, 0, nil, fmt.Errorf("merged audio is %.1fs, longer than the %.0fs limit (--max-duration)", dur.Seconds(), maxDur.Seconds())
	}
	if minRMS >= 0 && float64(maxRMS) > loudnessRatio*float64(max(minRMS, 1)) {
		warnings = append(warnings, fmt.Sprintf("loudness differs: %s (RMS %d) is much louder than %s (RMS %d); audio is not normalized", maxLabel, maxRMS, minLabel, minRMS))
	}
	return pcm, rate, warnings, nil
}

func joinTranscripts(parts []mergePart) (string, []string) {
	var texts, keyterms []string
	seen := map[string]bool{}
	for _, p := range parts {
		if t := strings.TrimSpace(p.transcript); t != "" {
			texts = append(texts, t)
		}
		for _, k := range p.keyterms {
			if !seen[k] {
				seen[k] = true
				keyterms = append(keyterms, k)
			}
		}
	}
	return strings.Join(texts, " "), keyterms
}

type mergeOptions struct {
	chunks      bool
	force       bool
	ownVoice    bool
	purpose     string
	gap, maxDur time.Duration
}

func newMergeCommand(d deps.Dependencies, open func() (*Store, error)) *cobra.Command {
	o := mergeOptions{gap: DefaultMergeGap, maxDur: DefaultMergeMax}
	cmd := &cobra.Command{
		Use:   "merge ID SOURCE...",
		Short: "Merge samples or chunks into one longer sample",
		Long: "Join two or more samples (or, with --chunks, recent chunks) in the given order into a new sample.\n" +
			"Parts are separated by --gap silence and must share sample rate and format; nothing is resampled or normalized.\n" +
			"The joined transcript opens in $VISUAL/$EDITOR for review. Inputs are never changed; the new sample's\n" +
			"source field lists them. Chunks must come from one session, be accepted, and lie at most 30s apart,\n" +
			"unless --force is given.\n\n" +
			"Example: voxi sample merge paragraph intro-1 intro-2\n" +
			"Example: voxi sample merge paragraph --chunks 12 13 14 --purpose voice",
		Args: cobra.MinimumNArgs(3),
		RunE: func(c *cobra.Command, a []string) error {
			return runMerge(c, d, open, o, a[0], a[1:])
		},
	}
	cmd.Flags().BoolVar(&o.chunks, "chunks", false, "sources are chunk indices from `voxi chunks list`")
	cmd.Flags().BoolVar(&o.force, "force", false, "allow chunks from different sessions, far apart, or rejected")
	cmd.Flags().BoolVar(&o.ownVoice, "own-voice", false, ownVoiceFlagUsage)
	cmd.Flags().StringVar(&o.purpose, "purpose", "", "purpose of the new sample (default: the sources' purpose, dictation for chunks)")
	cmd.Flags().DurationVar(&o.gap, "gap", o.gap, "silence between parts")
	cmd.Flags().DurationVar(&o.maxDur, "max-duration", o.maxDur, "refuse merged audio longer than this")
	return cmd
}

func runMerge(c *cobra.Command, d deps.Dependencies, open func() (*Store, error), o mergeOptions, id string, sources []string) error {
	if !validID(id) {
		return fmt.Errorf("invalid sample id %q", id)
	}
	if o.gap < 0 || o.gap > 5*time.Second {
		return fmt.Errorf("--gap %s out of range (0 to 5s)", o.gap)
	}
	seen := map[string]bool{}
	for _, src := range sources {
		if seen[src] {
			return fmt.Errorf("source %q given twice", src)
		}
		seen[src] = true
	}
	s, err := open()
	if err != nil {
		return err
	}
	if exists, err := s.Has(id); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("sample id %q already exists", id)
	}
	var parts []mergePart
	var purpose Purpose
	var consent *time.Time
	var provenance string
	if o.chunks {
		parts, provenance, err = chunkParts(d, sources, o.force)
		purpose = Dictation
	} else {
		parts, provenance, purpose, consent, err = sampleParts(s, sources)
	}
	if err != nil {
		return err
	}
	if o.purpose != "" {
		if !validPurpose(Purpose(o.purpose)) {
			return fmt.Errorf("invalid sample purpose %q (want dictation, noise, or voice)", o.purpose)
		}
		if Purpose(o.purpose) != purpose {
			consent = nil
		}
		purpose = Purpose(o.purpose)
	}
	pcm, rate, warnings, err := joinAudio(parts, o.gap, o.maxDur)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintf(c.ErrOrStderr(), "warning: %s\n", w)
	}
	in := stdinReader(d)
	if purpose == Voice && consent == nil {
		if err := confirmOwnVoice(d.Stdout, in, o.ownVoice); err != nil {
			return err
		}
		now := time.Now().UTC()
		consent = &now
	}
	text, keyterms := joinTranscripts(parts)
	if text != "" {
		if text, err = editTranscript(c.Context(), d, text); err != nil {
			return err
		}
		text = strings.TrimSpace(text)
	}
	if purpose != Noise && text == "" {
		return errors.New("sample transcript must not be empty")
	}
	tmp, err := os.MkdirTemp("", "voxi-merge-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	wav := filepath.Join(tmp, id+".wav")
	if err := audio.WriteWAVAudio(wav, pcm, rate); err != nil {
		return err
	}
	x := Sample{ID: id, Purpose: purpose, Transcript: text, Keyterms: keyterms, Created: time.Now(), Source: provenance, Consent: consent}
	if err := s.Put(x, wav); err != nil {
		return err
	}
	if d.Stdout != nil {
		fmt.Fprintf(d.Stdout, "merged %d parts into %s (%s, %.1fs)\n", len(parts), id, purpose, float64(len(pcm))/float64(2*rate))
	}
	return nil
}

// sampleParts resolves sample ids. All samples must share one purpose; the
// merged sample keeps it and, for voice, the latest consent of its parts.
func sampleParts(s *Store, ids []string) ([]mergePart, string, Purpose, *time.Time, error) {
	var parts []mergePart
	var samples []Sample
	var purpose Purpose
	var consent *time.Time
	for i, id := range ids {
		x, err := s.Get(id)
		if err != nil {
			return nil, "", "", nil, fmt.Errorf("sample %q: %w", id, err)
		}
		if i == 0 {
			purpose = x.Purpose
		} else if x.Purpose != purpose {
			return nil, "", "", nil, fmt.Errorf("sample %q is %s but %q is %s; merge samples of one purpose", id, x.Purpose, ids[0], purpose)
		}
		if x.Consent != nil && (consent == nil || x.Consent.After(*consent)) {
			consent = x.Consent
		}
		samples = append(samples, x)
		parts = append(parts, mergePart{label: "sample " + id, wav: s.AudioPath(x), transcript: x.Transcript, keyterms: x.Keyterms})
	}
	if purpose == Voice {
		if err := RequireConsent(samples); err != nil {
			return nil, "", "", nil, err
		}
	} else {
		consent = nil
	}
	return parts, "merge:samples:" + strings.Join(ids, ","), purpose, consent, nil
}

// chunkParts resolves chunk selectors and applies the adjacency guard.
func chunkParts(d deps.Dependencies, selectors []string, force bool) ([]mergePart, string, error) {
	if d.Getenv == nil {
		return nil, "", errors.New("chunk storage environment is unavailable")
	}
	b := chunks.NewBuffer(chunks.StorageDir(d.Getenv("XDG_RUNTIME_DIR"), d.Getenv("HOME")), chunks.DefaultBufferSize)
	var parts []mergePart
	var indices []string
	var first, prev chunks.Chunk
	for i, sel := range selectors {
		ch, err := b.Get(sel)
		if err != nil {
			return nil, "", fmt.Errorf("chunk %s: %w", sel, err)
		}
		if !force {
			if !ch.Accepted || ch.RejectionReason != "" {
				return nil, "", fmt.Errorf("chunk %d was rejected (%s); use --force to merge it anyway", ch.Index, ch.RejectionReason)
			}
			if i > 0 && ch.SessionID != first.SessionID {
				return nil, "", fmt.Errorf("chunk %d is from session %q, chunk %d from %q; use --force to merge across sessions", ch.Index, ch.SessionID, first.Index, first.SessionID)
			}
			if i > 0 {
				if pause := ch.Timestamp.Sub(prev.Timestamp); pause > maxChunkGap || pause < -maxChunkGap {
					return nil, "", fmt.Errorf("chunks %d and %d are %s apart (limit %s); use --force to merge them", prev.Index, ch.Index, pause.Round(time.Second), maxChunkGap)
				}
			}
		}
		if i == 0 {
			first = ch
		}
		prev = ch
		text := ch.CleanedTranscript
		if text == "" {
			text = ch.RawTranscript
		}
		parts = append(parts, mergePart{label: fmt.Sprintf("chunk %d", ch.Index), wav: b.WAVPath(ch), transcript: text})
		indices = append(indices, fmt.Sprintf("%s/%d", ch.SessionID, ch.Index))
	}
	return parts, "merge:chunks:" + strings.Join(indices, ","), nil
}
