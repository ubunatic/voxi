package debug

import (
	"testing"
)

func TestValidateMelMultiple(t *testing.T) {
	if err := ValidateMelMultiple("chunk", 0.48); err != nil {
		t.Fatalf("expected 0.48s to be valid, got: %v", err)
	}
	if err := ValidateMelMultiple("chunk", 0.50); err == nil {
		t.Fatalf("expected 0.50s to fail mel multiple check")
	}
}
