package install

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/spec"
)

func TestInstallVoxCPMRuntimePrefersVerifiedPrebuiltWithoutBuildTools(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "runtime.tar.gz")
	createTestRuntimeArchive(t, archive)
	digest, err := fileSHA256(archive)
	if err != nil {
		t.Fatal(err)
	}
	cfg := spec.TTSVoxCPMSpec{RuntimeAssets: []spec.TTSVoxCPMRuntimeAsset{
		{URL: "https://example.invalid/test.tar.gz", SHA256: digest},
		{URL: "https://example.invalid/portable-test.tar.gz", SHA256: digest},
	}}
	e := DefaultEffects()
	probeCount := 0
	e.Run = func(ctx context.Context, name string, args ...string) error {
		if name == "curl" {
			return copyFileAtomic(archive, args[len(args)-2], 0600)
		}
		if name == "tar" {
			return exec.CommandContext(ctx, name, args...).Run()
		}
		return fmt.Errorf("unexpected command %s", name)
	}
	e.RunOutput = func(_ context.Context, name string, args ...string) (string, error) {
		if !strings.HasSuffix(name, ".candidate") || len(args) != 1 || args[0] != "--list-devices" {
			return "", fmt.Errorf("unexpected probe %s %v", name, args)
		}
		probeCount++
		if probeCount == 1 {
			return "", fmt.Errorf("SIGILL")
		}
		return "Vulkan:0 test device", nil
	}
	e.LookPath = func(name string) (string, error) { return "", fmt.Errorf("%s unavailable", name) }
	destination := filepath.Join(t.TempDir(), "bin", "audiocpp_cli")
	if err := installVoxCPMRuntime(context.Background(), e, destination, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "test runtime" {
		t.Fatalf("installed runtime = %q, error %v", data, err)
	}
	if !verifiedInstalledRuntime(destination) {
		t.Fatal("installed runtime did not receive a valid verification marker")
	}
	if probeCount != 2 {
		t.Fatalf("runtime probe count = %d, want failed primary then working portable", probeCount)
	}
	if err := os.WriteFile(destination, []byte("changed"), 0755); err != nil {
		t.Fatal(err)
	}
	if verifiedInstalledRuntime(destination) {
		t.Fatal("modified runtime passed marker verification")
	}
}

func TestVulkanProbeRejectsCPUOnlyRuntime(t *testing.T) {
	e := DefaultEffects()
	e.RunOutput = func(context.Context, string, ...string) (string, error) { return "CPU:0 test cpu", nil }
	err := probeVoxCPMVulkan(context.Background(), e, "/tmp/audiocpp_cli")
	if err == nil || !strings.Contains(err.Error(), "no Vulkan device reported") {
		t.Fatalf("CPU-only runtime probe error = %v", err)
	}
}

func TestDownloadVerifiedRejectsChecksumMismatchAndReplacesPartial(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(destination, []byte("wrong existing content"), 0600); err != nil {
		t.Fatal(err)
	}
	e := DefaultEffects()
	e.Run = func(_ context.Context, name string, args ...string) error {
		if name != "curl" || len(args) < 2 {
			return fmt.Errorf("unexpected command %s %v", name, args)
		}
		return os.WriteFile(args[len(args)-2], []byte("model bytes"), 0600)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("model bytes")))
	if err := downloadVerified(context.Background(), e, "https://example.invalid/model", destination, digest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "model bytes" {
		t.Fatalf("verified model = %q, error %v", data, err)
	}
	err = downloadVerified(context.Background(), e, "https://example.invalid/model", destination, strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("mismatched download error = %v", err)
	}
	if _, err := os.Stat(destination + ".partial"); !os.IsNotExist(err) {
		t.Fatalf("partial download remains after checksum failure: %v", err)
	}
}

func createTestRuntimeArchive(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	tw := tar.NewWriter(zw)
	data := []byte("test runtime")
	if err := tw.WriteHeader(&tar.Header{Name: "package/bin/audiocpp_cli", Mode: 0755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
