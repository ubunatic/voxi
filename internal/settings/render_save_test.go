package settings

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRenderSaveSuccessWithRestart(t *testing.T) {
	home := "/home/testuser"
	if got := RenderSaveSuccessWithRestart(home, RestartRequested, nil); !strings.Contains(got, "restart requested") {
		t.Fatalf("successful restart output missing confirmation: %s", got)
	}

	if got := RenderSaveSuccessWithRestart(home, RestartFailed, errors.New("service unavailable")); !strings.Contains(got, "Run: systemctl --user try-restart voxi-agent.service") {
		t.Fatalf("failed restart output missing fallback: %s", got)
	}
}

func TestConfirmRestart(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  bool
	}{
		{name: "yes", input: "y\n", want: true},
		{name: "uppercase yes", input: "Y\n", want: true},
		{name: "no", input: "n\n"},
		{name: "enter", input: "\n"},
		{name: "eof", input: "", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var prompt bytes.Buffer
			if got := confirmRestart(strings.NewReader(test.input), &prompt); got != test.want {
				t.Fatalf("confirmRestart() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRenderSaveSuccessKeepsManualInstruction(t *testing.T) {
	got := RenderSaveSuccess("/home/testuser")
	if strings.Contains(got, "service restarted") || !strings.Contains(got, "try-restart") {
		t.Fatalf("legacy output incorrectly claims restart: %s", got)
	}
}
