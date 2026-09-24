package runtimepath

import "testing"

func TestUserDir(t *testing.T) {
	if got := UserDir("/custom/run", 42); got != "/custom/run" {
		t.Fatalf("UserDir(custom) = %q", got)
	}
	if got := UserDir("", 42); got != "/run/user/42" {
		t.Fatalf("UserDir(fallback) = %q", got)
	}
}

func TestVoxiDir(t *testing.T) {
	if got := VoxiDir("/run/user/42", 42); got != "/run/user/42/voxi" {
		t.Fatalf("VoxiDir() = %q", got)
	}
}
