// Package chunks manages a bounded ring buffer of recorded audio chunks (.wav)
// and their transcription metadata, kept in a persistent directory ($XDG_RUNTIME_DIR/voxi/chunks
// or fallback cache / temp directory).
package chunks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ubunatic.com/voxi/internal/audio"
)

const (
	// DefaultBufferSize is the number of chunks shown by `voxi chunks list`.
	DefaultBufferSize = 10
	// DefaultStorageCapacity is the number of chunks kept on disk before physical deletion.
	// Chunks beyond DefaultBufferSize but within DefaultStorageCapacity are shadow-deleted:
	// they remain on disk and in the manifest but are hidden from the list view, making them
	// available for post-hoc debugging (e.g. replaying audio to reproduce transcription bugs).
	DefaultStorageCapacity = 100
	manifestFileName       = "manifest.json"
)

// ReplacementSummary records a single deterministic replacement applied to a chunk.
type ReplacementSummary struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// LLMCleanupRecord captures LLM post-processing telemetry for a chunk.
type LLMCleanupRecord struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model,omitempty"`
	Output  string `json:"output,omitempty"`
	// FallbackReason names why the ASR text was kept instead of a cleaned
	// one, so a degraded cleanup server is distinguishable from a disabled or
	// merely no-op one (issue 115). Empty on success.
	FallbackReason string `json:"fallback_reason,omitempty"`
	Modified       bool   `json:"modified"`
}

// Chunk represents one recorded audio slice and its transcription diagnostics.
type Chunk struct {
	Index                  int                  `json:"index"`
	Timestamp              time.Time            `json:"timestamp"`
	SessionID              string               `json:"session_id,omitempty"`
	ChunkID                string               `json:"chunk_id,omitempty"`
	FinalizedAt            time.Time            `json:"finalized_at,omitempty"`
	TranscriptionStartedAt time.Time            `json:"transcription_started_at,omitempty"`
	TranscriptionEndedAt   time.Time            `json:"transcription_ended_at,omitempty"`
	TypingStartedAt        time.Time            `json:"typing_started_at,omitempty"`
	TypingEndedAt          time.Time            `json:"typing_ended_at,omitempty"`
	AudioDurationSecs      float64              `json:"audio_duration_secs"`
	PCMBytes               int                  `json:"pcm_bytes,omitempty"`
	MeanRMS                int                  `json:"mean_rms,omitempty"`
	PeakRMS                int                  `json:"peak_rms,omitempty"`
	VolumeSparkline        string               `json:"volume_sparkline,omitempty"` // fixed-width Braille level-over-time sparkline, precomputed at finalize time
	VoicedRatio            float64              `json:"voiced_ratio,omitempty"`
	ProbableSilence        bool                 `json:"probable_silence"`
	TranscribeDurationSec  float64              `json:"transcribe_duration_secs"`
	TranscriptWordCount    int                  `json:"transcript_word_count,omitempty"`
	RTF                    float64              `json:"rtf"`
	RawTranscript          string               `json:"raw_transcript"`
	CleanedTranscript      string               `json:"cleaned_transcript"`
	Accepted               bool                 `json:"accepted"`
	RejectionReason        string               `json:"rejection_reason,omitempty"`
	TranscriptChars        int                  `json:"transcript_chars,omitempty"`
	TranscriptDigest       string               `json:"transcript_digest,omitempty"`
	RepeatUnit             string               `json:"repeat_unit,omitempty"`
	RepeatCount            int                  `json:"repeat_count,omitempty"`
	Model                  string               `json:"model,omitempty"`
	Engine                 string               `json:"engine,omitempty"`
	AppliedReplacements    []ReplacementSummary `json:"applied_replacements,omitempty"`
	LLMCleanup             *LLMCleanupRecord    `json:"llm_cleanup,omitempty"`
	StopWordsMatched       []string             `json:"stop_words_matched,omitempty"`
	WAVFile                string               `json:"wav_file"` // relative filename in chunks dir, e.g. "chunk_0001.wav"
}

// Manifest is the serialized list of chunks currently in the ring buffer.
type Manifest struct {
	NextIndex int     `json:"next_index,omitempty"`
	Chunks    []Chunk `json:"chunks"`
}

// StorageDir determines the storage path for chunks:
// 1. $XDG_RUNTIME_DIR/voxi/chunks
// 2. /run/user/<uid>/voxi/chunks (if exists or can be created)
// 3. $HOME/.cache/voxi/chunks
// 4. filepath.Join(os.TempDir(), fmt.Sprintf("voxi-chunks-%d", os.Getuid()))
func StorageDir(xdgRuntimeDir, homeDir string) string {
	if xdgRuntimeDir != "" {
		return filepath.Join(xdgRuntimeDir, "voxi", "chunks")
	}
	uid := os.Getuid()
	runUser := fmt.Sprintf("/run/user/%d", uid)
	if fi, err := os.Stat(runUser); err == nil && fi.IsDir() {
		return filepath.Join(runUser, "voxi", "chunks")
	}
	if homeDir != "" {
		return filepath.Join(homeDir, ".cache", "voxi", "chunks")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("voxi-chunks-%d", uid))
}

// Buffer manages saving, reading, listing, and pruning chunks on disk.
//
// Two capacity limits govern the ring buffer:
//   - capacity: the number of chunks shown by List(); older chunks are hidden ("shadow-deleted")
//   - storageCapacity: the number of chunks kept on disk; chunks beyond this limit are physically deleted
//
// This separation lets the system retain a large on-disk history for debugging while exposing only
// the most recent chunks in the list view.
type Buffer struct {
	dir             string
	capacity        int // list/show view limit
	storageCapacity int // physical delete threshold
	mu              sync.Mutex
}

// NewBuffer returns a Buffer operating on dir with the given list capacity.
// If capacity <= 0, DefaultBufferSize (10) is used.
// The physical storage capacity is set to DefaultStorageCapacity (100).
// Use WithStorageCapacity to override it.
func NewBuffer(dir string, capacity int) *Buffer {
	if capacity <= 0 {
		capacity = DefaultBufferSize
	}
	storageCapacity := DefaultStorageCapacity
	if storageCapacity < capacity {
		storageCapacity = capacity
	}
	return &Buffer{
		dir:             dir,
		capacity:        capacity,
		storageCapacity: storageCapacity,
	}
}

// WithStorageCapacity returns a copy of the buffer with a custom physical storage capacity.
// storageCapacity must be >= capacity; if smaller, it is clamped to capacity.
func (b *Buffer) WithStorageCapacity(storageCapacity int) *Buffer {
	if storageCapacity < b.capacity {
		storageCapacity = b.capacity
	}
	return &Buffer{
		dir:             b.dir,
		capacity:        b.capacity,
		storageCapacity: storageCapacity,
		mu:              sync.Mutex{},
	}
}

// DefaultBuffer returns a Buffer using StorageDir(os.Getenv("XDG_RUNTIME_DIR"), os.Getenv("HOME")).
func DefaultBuffer() *Buffer {
	return NewBuffer(StorageDir(os.Getenv("XDG_RUNTIME_DIR"), os.Getenv("HOME")), DefaultBufferSize)
}

// Dir returns the directory where chunks are stored.
func (b *Buffer) Dir() string {
	return b.dir
}

// Capacity returns the number of chunks shown by List().
// Older chunks beyond this limit are shadow-deleted (still on disk, hidden from the list view).
func (b *Buffer) Capacity() int {
	return b.capacity
}

// StorageCapacity returns the physical storage limit.
// Chunks beyond this limit are permanently deleted from disk.
func (b *Buffer) StorageCapacity() int {
	return b.storageCapacity
}

// ensureDir ensures the chunks directory exists with 0700 permissions.
func (b *Buffer) ensureDir() error {
	if err := os.MkdirAll(b.dir, 0700); err != nil {
		return fmt.Errorf("create chunks dir: %w", err)
	}
	return os.Chmod(b.dir, 0700)
}

// manifestPath returns the absolute path to manifest.json.
func (b *Buffer) manifestPath() string {
	return filepath.Join(b.dir, manifestFileName)
}

// WAVPath returns the absolute path to a chunk's WAV file.
func (b *Buffer) WAVPath(c Chunk) string {
	if filepath.IsAbs(c.WAVFile) {
		return c.WAVFile
	}
	return filepath.Join(b.dir, c.WAVFile)
}

// loadManifestLocked loads and parses manifest.json without acquiring the lock.
func (b *Buffer) loadManifestLocked() (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(b.manifestPath())
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return m, fmt.Errorf("read chunk manifest: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse chunk manifest: %w", err)
	}
	return m, nil
}

// saveManifestLocked serializes and atomically writes manifest.json with 0600 permissions.
func (b *Buffer) saveManifestLocked(m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal chunk manifest: %w", err)
	}
	tmpFile, err := os.CreateTemp(b.dir, "manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp manifest: %w", err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("chmod temp manifest: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp manifest: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp manifest: %w", err)
	}

	dest := b.manifestPath()
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("rename manifest: %w", err)
	}
	return os.Chmod(dest, 0600)
}

// Add appends a new chunk with raw audio PCM bytes (16kHz mono S16_LE) or writes the WAV audio,
// updates manifest.json and per-chunk JSON sidecar, and prunes old chunks beyond capacity.
func (b *Buffer) nextIndexLocked(m *Manifest) int {
	if m.NextIndex <= 0 {
		maxIdx := 0
		for _, ch := range m.Chunks {
			if ch.Index > maxIdx {
				maxIdx = ch.Index
			}
		}
		m.NextIndex = maxIdx + 1
	}
	idx := m.NextIndex
	m.NextIndex++
	return idx
}

func (b *Buffer) Add(c Chunk, pcmAudio []byte, sampleRate int) (Chunk, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.ensureDir(); err != nil {
		return Chunk{}, err
	}

	m, _ := b.loadManifestLocked()
	c.Index = b.nextIndexLocked(&m)

	if c.Timestamp.IsZero() {
		c.Timestamp = time.Now()
	}
	wavName := fmt.Sprintf("chunk_%04d.wav", c.Index)
	wavPath := filepath.Join(b.dir, wavName)

	// Write audio if provided
	if len(pcmAudio) > 0 {
		if sampleRate <= 0 {
			sampleRate = 16000
		}
		tmpWav := filepath.Join(b.dir, fmt.Sprintf(".tmp_%04d.wav", c.Index))
		if err := audio.WriteWAVAudio(tmpWav, pcmAudio, sampleRate); err != nil {
			return Chunk{}, fmt.Errorf("write chunk audio: %w", err)
		}
		_ = os.Chmod(tmpWav, 0600)
		if err := os.Rename(tmpWav, wavPath); err != nil {
			_ = os.Remove(tmpWav)
			return Chunk{}, fmt.Errorf("finalize chunk wav: %w", err)
		}
	}
	c.WAVFile = wavName

	// Write per-chunk JSON sidecar
	sidecarName := fmt.Sprintf("chunk_%04d.json", c.Index)
	sidecarPath := filepath.Join(b.dir, sidecarName)
	if sidecarData, err := json.MarshalIndent(c, "", "  "); err == nil {
		_ = os.WriteFile(sidecarPath, sidecarData, 0600)
	}

	m.Chunks = append(m.Chunks, c)

	// Physically prune chunks that exceed the storage capacity.
	// Chunks beyond capacity but within storageCapacity are shadow-deleted:
	// they remain on disk and are simply dropped from List() output.
	if len(m.Chunks) > b.storageCapacity {
		excess := len(m.Chunks) - b.storageCapacity
		toPrune := m.Chunks[:excess]
		m.Chunks = m.Chunks[excess:]

		for _, old := range toPrune {
			_ = os.Remove(filepath.Join(b.dir, old.WAVFile))
			_ = os.Remove(filepath.Join(b.dir, fmt.Sprintf("chunk_%04d.json", old.Index)))
		}
	}

	if err := b.saveManifestLocked(m); err != nil {
		return Chunk{}, err
	}

	return c, nil
}

// AddExistingWAV adds a chunk whose WAV file already exists on disk (e.g. temporary WAV file).
// The source WAV file is copied or moved into the buffer directory.
func (b *Buffer) AddExistingWAV(c Chunk, srcWAVPath string, move bool) (Chunk, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.ensureDir(); err != nil {
		return Chunk{}, err
	}

	m, _ := b.loadManifestLocked()
	c.Index = b.nextIndexLocked(&m)

	if c.Timestamp.IsZero() {
		c.Timestamp = time.Now()
	}
	wavName := fmt.Sprintf("chunk_%04d.wav", c.Index)
	wavDest := filepath.Join(b.dir, wavName)

	if move {
		if err := os.Rename(srcWAVPath, wavDest); err != nil {
			// If rename fails (e.g. cross-device), copy and remove
			if err := copyFile(srcWAVPath, wavDest); err != nil {
				return Chunk{}, fmt.Errorf("copy chunk audio: %w", err)
			}
			_ = os.Remove(srcWAVPath)
		}
	} else {
		if err := copyFile(srcWAVPath, wavDest); err != nil {
			return Chunk{}, fmt.Errorf("copy chunk audio: %w", err)
		}
	}
	_ = os.Chmod(wavDest, 0600)
	c.WAVFile = wavName

	// Write sidecar JSON
	sidecarName := fmt.Sprintf("chunk_%04d.json", c.Index)
	sidecarPath := filepath.Join(b.dir, sidecarName)
	if sidecarData, err := json.MarshalIndent(c, "", "  "); err == nil {
		_ = os.WriteFile(sidecarPath, sidecarData, 0600)
	}

	m.Chunks = append(m.Chunks, c)

	if len(m.Chunks) > b.storageCapacity {
		excess := len(m.Chunks) - b.storageCapacity
		toPrune := m.Chunks[:excess]
		m.Chunks = m.Chunks[excess:]

		for _, old := range toPrune {
			_ = os.Remove(filepath.Join(b.dir, old.WAVFile))
			_ = os.Remove(filepath.Join(b.dir, fmt.Sprintf("chunk_%04d.json", old.Index)))
		}
	}

	if err := b.saveManifestLocked(m); err != nil {
		return Chunk{}, err
	}

	return c, nil
}

// Update replaces metadata for an existing chunk selected by its stable
// correlation ID while preserving its ring-buffer index and WAV filename.
func (b *Buffer) Update(c Chunk) (Chunk, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	m, err := b.loadManifestLocked()
	if err != nil {
		return Chunk{}, err
	}
	for i, existing := range m.Chunks {
		if c.ChunkID == "" || existing.ChunkID != c.ChunkID {
			continue
		}
		c.Index = existing.Index
		c.WAVFile = existing.WAVFile
		m.Chunks[i] = c
		sidecarPath := filepath.Join(b.dir, fmt.Sprintf("chunk_%04d.json", c.Index))
		data, marshalErr := json.MarshalIndent(c, "", "  ")
		if marshalErr != nil {
			return Chunk{}, fmt.Errorf("marshal chunk metadata: %w", marshalErr)
		}
		if err := os.WriteFile(sidecarPath, data, 0600); err != nil {
			return Chunk{}, fmt.Errorf("write chunk metadata: %w", err)
		}
		if err := b.saveManifestLocked(m); err != nil {
			return Chunk{}, err
		}
		return c, nil
	}
	return Chunk{}, fmt.Errorf("chunk correlation ID %q not found", c.ChunkID)
}

// List returns the most recent chunks up to the buffer's capacity, ordered oldest-first
// (or newest-first if reverse is true). Chunks beyond capacity that are still on disk
// (shadow-deleted) are not included; use Get with an explicit index to access them.
func (b *Buffer) List(reverse bool) ([]Chunk, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	m, err := b.loadManifestLocked()
	if err != nil {
		return nil, err
	}
	all := m.Chunks
	// Apply the list-view capacity window: show only the most recent `capacity` entries.
	if len(all) > b.capacity {
		all = all[len(all)-b.capacity:]
	}
	chunks := make([]Chunk, len(all))
	copy(chunks, all)

	if reverse {
		for i, j := 0, len(chunks)-1; i < j; i, j = i+1, j-1 {
			chunks[i], chunks[j] = chunks[j], chunks[i]
		}
	}
	return chunks, nil
}

// Get returns the chunk by index (or last if selector is "last" or empty).
func (b *Buffer) Get(selector string) (Chunk, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	m, err := b.loadManifestLocked()
	if err != nil {
		return Chunk{}, err
	}
	if len(m.Chunks) == 0 {
		return Chunk{}, fmt.Errorf("no chunks recorded yet")
	}

	selector = strings.TrimSpace(selector)
	if selector == "" || selector == "last" {
		return m.Chunks[len(m.Chunks)-1], nil
	}

	idx, err := strconv.Atoi(selector)
	if err != nil {
		return Chunk{}, fmt.Errorf("invalid chunk selector %q (want integer index or 'last')", selector)
	}

	for _, c := range m.Chunks {
		if c.Index == idx {
			return c, nil
		}
	}
	return Chunk{}, fmt.Errorf("chunk %d not found in recent chunks buffer", idx)
}

// ReadWAV reads and returns the audio bytes of the chunk's WAV file.
func (b *Buffer) ReadWAV(c Chunk) ([]byte, error) {
	path := b.WAVPath(c)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read chunk wav (%s): %w", path, err)
	}
	return data, nil
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0600)
}

// PruneUnreferenced removes any .wav or .json files in dir that are not in the manifest.
func (b *Buffer) PruneUnreferenced() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	m, err := b.loadManifestLocked()
	if err != nil {
		return err
	}
	referenced := make(map[string]bool)
	referenced[manifestFileName] = true
	for _, c := range m.Chunks {
		referenced[c.WAVFile] = true
		referenced[fmt.Sprintf("chunk_%04d.json", c.Index)] = true
	}

	entries, err := os.ReadDir(b.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && !referenced[e.Name()] {
			_ = os.Remove(filepath.Join(b.dir, e.Name()))
		}
	}
	return nil
}

// SortChunks sorts chunks by Index ascending.
func SortChunks(chunks []Chunk) {
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Index < chunks[j].Index
	})
}
