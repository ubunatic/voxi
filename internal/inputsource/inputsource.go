// Package inputsource reads the desktop input source selected for typing.
package inputsource

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ubunatic.com/voxi/internal/deps"
)

// Source is the XKB layout and optional variant of an active input source.
type Source struct {
	Layout  string
	Variant string
}

var sourceTuplePattern = regexp.MustCompile(`\('([^']+)',\s*'([^']+)'\)`)

// DetectActive reads GNOME's active input source. It returns an error when
// gsettings is unavailable, malformed, or no active XKB source is present.
func DetectActive(ctx context.Context, d deps.Dependencies) (Source, error) {
	if d.RunOutput == nil {
		return Source{}, fmt.Errorf("output dependency is not configured")
	}
	value, err := d.RunOutput(ctx, "gsettings", "get", "org.gnome.desktop.input-sources", "sources")
	if err != nil {
		return Source{}, fmt.Errorf("read input sources: %w", err)
	}
	tuples := sourceTuplePattern.FindAllStringSubmatch(value, -1)
	if len(tuples) == 0 {
		return Source{}, fmt.Errorf("no input sources found")
	}
	currentValue, err := d.RunOutput(ctx, "gsettings", "get", "org.gnome.desktop.input-sources", "current")
	if err != nil {
		return Source{}, fmt.Errorf("read current input source: %w", err)
	}
	index, err := parseCurrentIndex(currentValue)
	if err != nil {
		return Source{}, err
	}
	if index < 0 || index >= len(tuples) {
		return Source{}, fmt.Errorf("current input source index %d out of range", index)
	}
	kind := tuples[index][1]
	id := tuples[index][2]
	if kind != "xkb" {
		return Source{}, fmt.Errorf("active input source %q is not an XKB layout", kind)
	}
	parts := strings.SplitN(id, "+", 2)
	source := Source{Layout: parts[0]}
	if len(parts) == 2 {
		source.Variant = parts[1]
	}
	if source.Layout == "" {
		return Source{}, fmt.Errorf("active input source has empty layout")
	}
	return source, nil
}

func parseCurrentIndex(value string) (int, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0, fmt.Errorf("invalid current input source %q", strings.TrimSpace(value))
	}
	raw := fields[len(fields)-1]
	index, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid current input source %q: %w", strings.TrimSpace(value), err)
	}
	return index, nil
}
