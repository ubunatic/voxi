package eager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteEagerChunkWAVRecreatesDeletedTempDir(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "daemon", "session")
	audio := []byte{1, 2, 3, 4}

	firstPath, err := writeEagerChunkWAV(tempDir, 1, audio)
	if err != nil {
		t.Fatalf("write first chunk: %v", err)
	}
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("stat first chunk: %v", err)
	}

	if err := os.RemoveAll(tempDir); err != nil {
		t.Fatalf("remove temp dir between chunks: %v", err)
	}

	secondPath, err := writeEagerChunkWAV(tempDir, 2, audio)
	if err != nil {
		t.Fatalf("write second chunk after temp dir removal: %v", err)
	}
	if _, err := os.Stat(secondPath); err != nil {
		t.Fatalf("stat second chunk: %v", err)
	}
}

func TestWriteEagerChunkWAVReportsTempDirReplacedByFile(t *testing.T) {
	baseDir := t.TempDir()
	tempDir := filepath.Join(baseDir, "session")
	if err := os.WriteFile(tempDir, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("replace temp dir with file: %v", err)
	}

	_, err := writeEagerChunkWAV(tempDir, 1, []byte{1, 2, 3, 4})
	if err == nil {
		t.Fatal("expected an error when the temp dir path is a regular file")
	}
	if !strings.Contains(err.Error(), "create eager audio temp directory") {
		t.Fatalf("error = %q, want clear temp directory context", err)
	}
}
