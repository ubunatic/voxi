package inputsource

import (
	"context"
	"errors"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

func TestDetectActive(t *testing.T) {
	tests := []struct {
		name, sources, current string
		want                   Source
	}{
		{"de variant", "[('xkb', 'de+nodeadkeys'), ('xkb', 'gb+mac')]", "0", Source{"de", "nodeadkeys"}},
		{"gb variant", "[('xkb', 'de+nodeadkeys'), ('xkb', 'gb+mac')]", "1", Source{"gb", "mac"}},
		{"us variant with uint32 prefix", "[('xkb', 'de+nodeadkeys'), ('xkb', 'gb+mac'), ('xkb', 'us+mac-iso')]", "uint32 2", Source{"us", "mac-iso"}},
		{"plain layout", "[('xkb', 'us')]", "uint32 0", Source{"us", ""}},
		{"mixed with non-xkb source", "[('ibus', 'libpinyin'), ('xkb', 'de+nodeadkeys')]", "uint32 1", Source{"de", "nodeadkeys"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := deps.Dependencies{RunOutput: func(_ context.Context, _ string, args ...string) (string, error) {
				if args[len(args)-1] == "sources" {
					return test.sources, nil
				}
				return test.current, nil
			}}
			got, err := DetectActive(context.Background(), d)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDetectActiveFailure(t *testing.T) {
	d := deps.Dependencies{RunOutput: func(context.Context, string, ...string) (string, error) { return "", errors.New("missing gsettings") }}
	if _, err := DetectActive(context.Background(), d); err == nil {
		t.Fatal("expected detection error")
	}
}
