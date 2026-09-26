package settings

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
)

func TestRestartAgent(t *testing.T) {
	for _, test := range []struct {
		name    string
		runErr  error
		wantErr bool
	}{
		{name: "success"},
		{name: "failure", runErr: errors.New("not running"), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			d := deps.Dependencies{Run: func(_ context.Context, name string, args ...string) error {
				called = true
				if name != "systemctl" || len(args) != 3 || args[0] != "--user" || args[1] != "try-restart" || args[2] != "voxi-agent.service" {
					t.Fatalf("unexpected command: %s %v", name, args)
				}
				return test.runErr
			}}
			if err := restartAgent(context.Background(), d); (err != nil) != test.wantErr {
				t.Fatalf("restartAgent() error = %v, wantErr %v", err, test.wantErr)
			}
			if !called {
				t.Fatal("expected restart command")
			}
		})
	}
}

func TestRestartServicesR2T2(t *testing.T) {
	var executed []string
	d := deps.Dependencies{Run: func(_ context.Context, name string, args ...string) error {
		executed = append(executed, name+" "+strings.Join(args, " "))
		return nil
	}}

	s := &config.UserSettings{ASRModel: "r2t2-confucius4"}
	if err := restartServices(context.Background(), d, "", s); err != nil {
		t.Fatalf("restartServices() error = %v", err)
	}

	joined := strings.Join(executed, "\n")
	if !strings.Contains(joined, "systemctl --user enable --now voxi-r2t2.service") {
		t.Errorf("expected R2T2 service enable --now, got:\n%s", joined)
	}
	if !strings.Contains(joined, "systemctl --user try-restart voxi-agent.service") {
		t.Errorf("expected voxi-agent.service try-restart, got:\n%s", joined)
	}
}

func TestRestartServicesNonR2T2(t *testing.T) {
	var executed []string
	d := deps.Dependencies{Run: func(_ context.Context, name string, args ...string) error {
		executed = append(executed, name+" "+strings.Join(args, " "))
		return nil
	}}

	s := &config.UserSettings{ASRModel: "cohere-transcribe-03-2026"}
	if err := restartServices(context.Background(), d, "", s); err != nil {
		t.Fatalf("restartServices() error = %v", err)
	}

	joined := strings.Join(executed, "\n")
	if !strings.Contains(joined, "systemctl --user disable --now voxi-r2t2.service") {
		t.Errorf("expected R2T2 service disable --now, got:\n%s", joined)
	}
	if !strings.Contains(joined, "systemctl --user try-restart voxi-agent.service") {
		t.Errorf("expected voxi-agent.service try-restart, got:\n%s", joined)
	}
}
