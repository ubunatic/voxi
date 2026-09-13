package chunks

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStorageDirFallback(t *testing.T) {
	d := StorageDir("/custom/runtime", "/home/test")
	if d != "/custom/runtime/voxi/chunks" {
		t.Fatalf("StorageDir with xdgRuntimeDir got %q, want %q", d, "/custom/runtime/voxi/chunks")
	}

	d2 := StorageDir("", "/home/test")
	// If /run/user/<uid> exists on linux it might pick that, or ~/.cache/voxi/chunks
	if !filepath.IsAbs(d2) {
		t.Fatalf("expected absolute path, got %q", d2)
	}
}

func TestRingBufferAddAndRotation(t *testing.T) {
	dir := t.TempDir()
	buf := NewBuffer(dir, 3)

	dummyPCM := make([]byte, 3200) // 0.1s at 16kHz mono S16_LE

	for i := 1; i <= 5; i++ {
		c := Chunk{
			Index:                 i,
			Timestamp:             time.Now(),
			AudioDurationSecs:     0.1,
			TranscribeDurationSec: 0.05,
			RTF:                   0.5,
			RawTranscript:         fmt.Sprintf("raw text %d", i),
			CleanedTranscript:     fmt.Sprintf("clean text %d", i),
			Accepted:              i%2 == 1,
			RejectionReason:       map[bool]string{true: "", false: "silence_artifact"}[i%2 == 1],
		}
		if _, err := buf.Add(c, dummyPCM, 16000); err != nil {
			t.Fatalf("Add chunk %d failed: %v", i, err)
		}
	}

	chunks, err := buf.List(false)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	// List returns only the last `capacity` (3) entries.
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Index != 3 || chunks[1].Index != 4 || chunks[2].Index != 5 {
		t.Fatalf("unexpected chunk indices: %v, %v, %v", chunks[0].Index, chunks[1].Index, chunks[2].Index)
	}

	// Shadow-deleted chunks (1 and 2) must still exist on disk (storageCapacity=100 by default).
	for i := 1; i <= 2; i++ {
		oldWav := filepath.Join(dir, fmt.Sprintf("chunk_%04d.wav", i))
		if _, err := os.Stat(oldWav); err != nil {
			t.Errorf("expected shadow-deleted %s to remain on disk: %v", oldWav, err)
		}
		oldSidecar := filepath.Join(dir, fmt.Sprintf("chunk_%04d.json", i))
		if _, err := os.Stat(oldSidecar); err != nil {
			t.Errorf("expected shadow-deleted %s to remain on disk: %v", oldSidecar, err)
		}
	}

	// Verify visible files exist
	for i := 3; i <= 5; i++ {
		wav := filepath.Join(dir, fmt.Sprintf("chunk_%04d.wav", i))
		if _, err := os.Stat(wav); err != nil {
			t.Errorf("expected %s to exist: %v", wav, err)
		}
		sidecar := filepath.Join(dir, fmt.Sprintf("chunk_%04d.json", i))
		if _, err := os.Stat(sidecar); err != nil {
			t.Errorf("expected %s to exist: %v", sidecar, err)
		}
	}

	// Test Get
	last, err := buf.Get("last")
	if err != nil {
		t.Fatalf("Get last failed: %v", err)
	}
	if last.Index != 5 {
		t.Fatalf("expected last index 5, got %d", last.Index)
	}

	c4, err := buf.Get("4")
	if err != nil {
		t.Fatalf("Get 4 failed: %v", err)
	}
	if c4.Index != 4 || c4.Accepted {
		t.Fatalf("unexpected c4: %+v", c4)
	}

	// Shadow-deleted chunks are still accessible via Get by index.
	c1, err := buf.Get("1")
	if err != nil {
		t.Fatalf("expected shadow-deleted chunk 1 to be accessible via Get, got error: %v", err)
	}
	if c1.Index != 1 {
		t.Fatalf("expected index 1, got %d", c1.Index)
	}

	// Test ReadWAV
	wavBytes, err := buf.ReadWAV(last)
	if err != nil {
		t.Fatalf("ReadWAV failed: %v", err)
	}
	if len(wavBytes) < 44 { // RIFF header size
		t.Fatalf("wavBytes too small: %d", len(wavBytes))
	}
}

// TestPhysicalPruning verifies that chunks are physically deleted once storageCapacity is exceeded.
func TestPhysicalPruning(t *testing.T) {
	dir := t.TempDir()
	// Use WithStorageCapacity to set a tight physical limit for this test.
	buf := NewBuffer(dir, 2).WithStorageCapacity(3)

	dummyPCM := make([]byte, 3200)

	for i := 1; i <= 4; i++ {
		c := Chunk{
			Timestamp:         time.Now(),
			AudioDurationSecs: 0.1,
			RawTranscript:     fmt.Sprintf("text %d", i),
			Accepted:          true,
		}
		if _, err := buf.Add(c, dummyPCM, 16000); err != nil {
			t.Fatalf("Add chunk %d failed: %v", i, err)
		}
	}

	// After 4 adds with storageCapacity=3, chunk 1 should be physically deleted.
	oldWav := filepath.Join(dir, "chunk_0001.wav")
	if _, err := os.Stat(oldWav); !os.IsNotExist(err) {
		t.Errorf("expected chunk_0001.wav to be physically deleted, but it exists")
	}

	// Chunks 2, 3, 4 should still be on disk.
	for i := 2; i <= 4; i++ {
		wav := filepath.Join(dir, fmt.Sprintf("chunk_%04d.wav", i))
		if _, err := os.Stat(wav); err != nil {
			t.Errorf("expected %s to exist: %v", wav, err)
		}
	}

	// List shows only last 2 (capacity=2).
	chunks, err := buf.List(false)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks in list, got %d", len(chunks))
	}
	if chunks[0].Index != 3 || chunks[1].Index != 4 {
		t.Fatalf("unexpected list indices: %d, %d", chunks[0].Index, chunks[1].Index)
	}
}

func TestAddExistingWAV(t *testing.T) {
	dir := t.TempDir()
	buf := NewBuffer(dir, 2)

	srcDir := t.TempDir()
	srcWAV := filepath.Join(srcDir, "test.wav")
	if err := os.WriteFile(srcWAV, []byte("RIFF1234WAVEfmt testdata"), 0600); err != nil {
		t.Fatal(err)
	}

	c := Chunk{
		Index:             1,
		RawTranscript:     "hello",
		CleanedTranscript: "hello",
		Accepted:          true,
	}
	added, err := buf.AddExistingWAV(c, srcWAV, false)
	if err != nil {
		t.Fatalf("AddExistingWAV failed: %v", err)
	}
	if added.WAVFile != "chunk_0001.wav" {
		t.Fatalf("unexpected wav file: %s", added.WAVFile)
	}
	// Verify source was not removed
	if _, err := os.Stat(srcWAV); err != nil {
		t.Fatalf("expected srcWAV to remain: %v", err)
	}

	// Now move another
	srcWAV2 := filepath.Join(srcDir, "test2.wav")
	if err := os.WriteFile(srcWAV2, []byte("RIFF1234WAVEfmt testdata2"), 0600); err != nil {
		t.Fatal(err)
	}
	c2 := Chunk{Index: 2, Accepted: true}
	_, err = buf.AddExistingWAV(c2, srcWAV2, true)
	if err != nil {
		t.Fatalf("AddExistingWAV move failed: %v", err)
	}
	if _, err := os.Stat(srcWAV2); !os.IsNotExist(err) {
		t.Fatalf("expected srcWAV2 to be removed after move")
	}
}

func TestUpdateUsesStableChunkCorrelation(t *testing.T) {
	dir := t.TempDir()
	buf := NewBuffer(dir, 2)
	added, err := buf.Add(Chunk{ChunkID: "session/1", CleanedTranscript: "hello", Accepted: true}, make([]byte, 640), 16000)
	if err != nil {
		t.Fatal(err)
	}
	added.TypingStartedAt = time.Unix(10, 0)
	added.TypingEndedAt = time.Unix(11, 0)
	updated, err := buf.Update(added)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Index != added.Index || updated.WAVFile != added.WAVFile {
		t.Fatalf("Update changed storage identity: before=%+v after=%+v", added, updated)
	}
	got, err := buf.Get("last")
	if err != nil {
		t.Fatal(err)
	}
	if !got.TypingStartedAt.Equal(time.Unix(10, 0)) || !got.TypingEndedAt.Equal(time.Unix(11, 0)) {
		t.Fatalf("typing timestamps not persisted: %+v", got)
	}
}

func TestRingBufferConcurrency(t *testing.T) {
	dir := t.TempDir()
	buf := NewBuffer(dir, 5)

	var wg sync.WaitGroup
	workers := 10
	iterations := 5

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				idx := workerID*iterations + i + 1
				c := Chunk{
					Index:             idx,
					Timestamp:         time.Now(),
					CleanedTranscript: fmt.Sprintf("text from worker %d it %d", workerID, i),
					Accepted:          true,
				}
				dummyPCM := make([]byte, 640)
				_, _ = buf.Add(c, dummyPCM, 16000)
				_, _ = buf.List(false)
				_, _ = buf.Get("last")
			}
		}(w)
	}

	wg.Wait()

	chunks, err := buf.List(false)
	if err != nil {
		t.Fatalf("List after concurrency failed: %v", err)
	}
	if len(chunks) > 5 {
		t.Fatalf("expected at most 5 chunks, got %d", len(chunks))
	}
}

func TestChunkJSONSerializationWithPipelineMetadata(t *testing.T) {
	c := Chunk{
		Index:               10,
		Model:               "cohere-transcribe-03-2026",
		Engine:              "cohere-transcribe",
		AppliedReplacements: []ReplacementSummary{{From: "Voxy", To: "voxi"}},
		LLMCleanup: &LLMCleanupRecord{
			Enabled:  true,
			Model:    "qwen3-4b-instruct-2507-q4",
			Output:   "Hello from Voxi",
			Modified: true,
		},
		StopWordsMatched: []string{"thank you"},
		Accepted:         true,
	}

	dir := t.TempDir()
	buf := NewBuffer(dir, 5)
	added, err := buf.Add(c, make([]byte, 640), 16000)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	got, err := buf.Get(fmt.Sprintf("%d", added.Index))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Model != "cohere-transcribe-03-2026" || got.Engine != "cohere-transcribe" {
		t.Errorf("model/engine mismatch: got model=%q engine=%q", got.Model, got.Engine)
	}
	if len(got.AppliedReplacements) != 1 || got.AppliedReplacements[0].From != "Voxy" || got.AppliedReplacements[0].To != "voxi" {
		t.Errorf("applied replacements mismatch: %+v", got.AppliedReplacements)
	}
	if got.LLMCleanup == nil || !got.LLMCleanup.Enabled || got.LLMCleanup.Model != "qwen3-4b-instruct-2507-q4" || !got.LLMCleanup.Modified {
		t.Errorf("llm cleanup mismatch: %+v", got.LLMCleanup)
	}
	if len(got.StopWordsMatched) != 1 || got.StopWordsMatched[0] != "thank you" {
		t.Errorf("stop words matched mismatch: %+v", got.StopWordsMatched)
	}
}
