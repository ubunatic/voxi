package eager

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/typing"
)

func TestDeliveryLedgerClaimsIdentityAtMostOnceAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eager-delivery-ledger.jsonl")
	first := newDeliveryLedger(path)
	claimed, err := first.Claim("session/1")
	if err != nil || !claimed {
		t.Fatalf("first Claim = %t, %v; want true", claimed, err)
	}
	if claimed, err := first.Claim("session/1"); err != nil || claimed {
		t.Fatalf("duplicate Claim = %t, %v; want false", claimed, err)
	}
	second := newDeliveryLedger(path)
	if claimed, err := second.Claim("session/1"); err != nil || claimed {
		t.Fatalf("restart Claim = %t, %v; want false", claimed, err)
	}
	if claimed, err := second.Claim("session/2"); err != nil || !claimed {
		t.Fatalf("new identity Claim = %t, %v; want true", claimed, err)
	}
}

type testInjectionObserver struct {
	startedPath string
	attempt     typing.InjectionAttempt
}

func (o *testInjectionObserver) Started(path string, _ time.Time)    { o.startedPath = path }
func (o *testInjectionObserver) Completed(a typing.InjectionAttempt) { o.attempt = a }

func TestTypeTextObservedReportsSingleProcessAttempt(t *testing.T) {
	observer := new(testInjectionObserver)
	d := deps.Dependencies{
		Getenv: func(string) string { return filepath.Join(t.TempDir(), "missing-pipe") },
		LookPath: func(name string) (string, error) {
			if name == "dotool" {
				return "/fake/dotool", nil
			}
			return "", errors.New("not found")
		},
		RunStdinProcess: func(ctx context.Context, stdin, name string, args ...string) (int, error) {
			if stdin == "" || name != "dotool" || len(args) != 0 {
				t.Fatalf("unexpected injector call: %q %q %v", stdin, name, args)
			}
			return 4242, ctx.Err()
		},
	}
	if err := typing.TypeTextObserved(context.Background(), d, "once", observer); err != nil {
		t.Fatalf("TypeTextObserved: %v", err)
	}
	if observer.startedPath != "dotool" || observer.attempt.Path != "dotool" || observer.attempt.PID != 4242 || observer.attempt.Err != nil {
		t.Fatalf("attempt = %+v, started path=%q", observer.attempt, observer.startedPath)
	}
}
