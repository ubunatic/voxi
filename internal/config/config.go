package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	typeDelayActiveRe = regexp.MustCompile(`(?m)^([ \t]*)type_delay_ms([ \t]*=[ \t]*)([0-9]+)[ \t]*$`)
	outputHeaderRe    = regexp.MustCompile(`(?m)^\[output\][ \t]*$`)
)

// VoxtypeConfigPath is the well-known location of Voxtype's user config.
func VoxtypeConfigPath(home string) string {
	return filepath.Join(home, ".config", "voxtype", "config.toml")
}

// ReadTypeDelayMs reads the active (uncommented) `type_delay_ms` value from
// a voxtype config.toml. Returns 0, false if the key is absent (voxtype's default).
func ReadTypeDelayMs(path string) (int, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read %s: %w", path, err)
	}
	m := typeDelayActiveRe.FindSubmatch(data)
	if m == nil {
		return 0, false, nil
	}
	ms, err := strconv.Atoi(string(m[3]))
	if err != nil {
		return 0, false, fmt.Errorf("parse type_delay_ms: %w", err)
	}
	return ms, true, nil
}

// SetTypeDelayMs writes `ms` into config.toml at `path`, preserving comments and unrelated settings.
func SetTypeDelayMs(path string, ms int) error {
	if ms < 0 {
		return fmt.Errorf("type_delay_ms must be >= 0, got %d", ms)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	replacement := []byte(fmt.Sprintf("${1}type_delay_ms${2}%d", ms))
	if typeDelayActiveRe.Match(data) {
		updated := typeDelayActiveRe.ReplaceAll(data, replacement)
		return writeConfigAtomic(path, updated)
	}
	loc := outputHeaderRe.FindIndex(data)
	if loc == nil {
		return fmt.Errorf("no [output] section found in %s; cannot add type_delay_ms", path)
	}
	insertAt := loc[1]
	line := []byte(fmt.Sprintf("\ntype_delay_ms = %d", ms))
	updated := append(append(append([]byte{}, data[:insertAt]...), line...), data[insertAt:]...)
	return writeConfigAtomic(path, updated)
}

func writeConfigAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	mode := os.FileMode(0644)
	if err == nil {
		mode = info.Mode()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Chmod(mode)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("install config: %w", err)
	}
	return nil
}
