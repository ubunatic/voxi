package spec

import (
	"testing"
	"time"
)

func TestLoadDoctor(t *testing.T) {
	s, err := LoadDoctor()
	if err != nil {
		t.Fatalf("LoadDoctor() error = %v", err)
	}

	if len(s.Checks) == 0 {
		t.Fatal("expected at least one doctor check")
	}

	if s.SnapshotKeepCount() < 1 {
		t.Fatalf("SnapshotKeepCount() = %d, want >= 1", s.SnapshotKeepCount())
	}

	if s.SnapshotMaxAge() < 24*time.Hour {
		t.Fatalf("SnapshotMaxAge() = %v, want >= 24h", s.SnapshotMaxAge())
	}
}

func TestValidateDoctorSpecSnapshots(t *testing.T) {
	s, err := LoadDoctor()
	if err != nil {
		t.Fatalf("LoadDoctor() error = %v", err)
	}

	badCount := *s
	badCount.Snapshots.KeepCount = 0
	if err := validateDoctorSpec(&badCount); err == nil {
		t.Fatal("expected error for keep_count = 0")
	}

	badAge := *s
	badAge.Snapshots.MaxAgeDays = 0
	if err := validateDoctorSpec(&badAge); err == nil {
		t.Fatal("expected error for max_age_days = 0")
	}
}
