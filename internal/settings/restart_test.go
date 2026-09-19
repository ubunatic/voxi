package settings

import (
	"context"
	"errors"
	"testing"

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
