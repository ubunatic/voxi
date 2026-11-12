package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/spec"
)

// r2t2FallbackDown makes the voxi R2T2 endpoint refuse connections, so the
// probe reports a fault and the R2T2 repair is considered.
func r2t2FallbackDown() *fakeASRProbe {
	return &fakeASRProbe{
		answers: map[string]string{pinnedR2T2URL: "One, two."},
		errs:    map[string]error{fallbackR2T2URL: errors.New("dial tcp 127.0.0.1:18131: connect: connection refused")},
	}
}

// withWhich makes `which llama-server` succeed or fail on the host.
func withWhich(h *fixtureHost, found bool) *fixtureHost {
	prev := h.run
	h.run = func(name string, args ...string) commandResult {
		if name == "which" {
			if found {
				return commandResult{output: "/usr/bin/llama-server\n"}
			}
			return commandResult{err: errors.New("exit status 1")}
		}
		return prev(name, args...)
	}
	return h
}

func TestR2T2FallbackDownOffersInstallRepair(t *testing.T) {
	s := loadDoctorSpec(t)
	check := checkFor(t, s, "asr-engine")
	home := t.TempDir()
	t.Setenv("HOME", home)
	saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4-voxi"})

	host := withWhich(psHost(""), true)
	res := HostDoctor{Host: host, Spec: s, ASRProbe: r2t2FallbackDown().probe}.checkAsrEngine(context.Background(), check)
	if res.Status != FAULT {
		t.Fatalf("got %+v, want FAULT", res)
	}
	if !slices.Equal(res.Repairs, []string{"voxi install --r2t2 --yes"}) {
		t.Fatalf("repairs = %v, want the R2T2 install repair", res.Repairs)
	}
	if !strings.Contains(res.Detail, "model files missing") {
		t.Fatalf("detail does not name the missing files: %q", res.Detail)
	}
	if notice := strings.Join(res.Notice, "\n"); !strings.Contains(notice, "GiB") || !strings.Contains(notice, "sha256") {
		t.Fatalf("notice does not show the download size and sha256 check: %q", notice)
	}
}

func TestR2T2FallbackWithoutLlamaServerIsGuidanceOnly(t *testing.T) {
	s := loadDoctorSpec(t)
	check := checkFor(t, s, "asr-engine")
	home := t.TempDir()
	t.Setenv("HOME", home)
	saveASRSettings(t, home, &config.UserSettings{ASRModel: "r2t2-confucius4-voxi"})

	host := withWhich(psHost(""), false)
	res := HostDoctor{Host: host, Spec: s, ASRProbe: r2t2FallbackDown().probe}.checkAsrEngine(context.Background(), check)
	if res.Status != FAULT || len(res.Repairs) != 0 {
		t.Fatalf("got %+v, want FAULT with no repair", res)
	}
	if !strings.Contains(strings.Join(res.See, "\n"), "llama_server_path") {
		t.Fatalf("missing llama-server guidance: %v", res.See)
	}
}

// missingR2T2Files compares sizes against the spec. A sparse file of the
// spec's size counts as present; the sha256 check is left to the install repair.
func TestMissingR2T2FilesUsesSpecSizes(t *testing.T) {
	models, err := spec.LoadModels()
	if err != nil {
		t.Fatal(err)
	}
	files := models.Models["r2t2-confucius4-voxi"].Files
	if len(files) != 2 {
		t.Fatalf("spec lists %d R2T2 files, want 2", len(files))
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".cache", "voxi", "models")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, files[0].Name))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(files[0].SizeBytes); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	// A truncated file of the wrong size is missing too.
	if err := os.WriteFile(filepath.Join(dir, files[1].Name), []byte("short"), 0644); err != nil {
		t.Fatal(err)
	}
	missing, total := missingR2T2Files(files)
	if !slices.Equal(missing, []string{files[1].Name}) {
		t.Fatalf("missing = %v, want only %s", missing, files[1].Name)
	}
	if total != files[1].SizeBytes {
		t.Fatalf("total = %d, want %d", total, files[1].SizeBytes)
	}
}
