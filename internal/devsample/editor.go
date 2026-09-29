package devsample

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ubunatic.com/voxi/internal/deps"
)

// editorOffValue in VOXI_SAMPLE_EDITOR disables the external editor and keeps
// the inline line-editor prompt.
const editorOffValue = "off"

// editTranscriptFn runs the user's editor on the transcript; tests replace it.
var editTranscriptFn = editTranscriptInEditor

// resolveEditor returns the editor command from $VISUAL, then $EDITOR, or ""
// when none is configured or VOXI_SAMPLE_EDITOR=off.
func resolveEditor(d deps.Dependencies) string {
	if d.Getenv == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(d.Getenv("VOXI_SAMPLE_EDITOR")), editorOffValue) {
		return ""
	}
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(d.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

// promptTranscript asks for the corrected transcript, using the user's editor
// when one is configured and stdin is a terminal, and the inline prompt otherwise.
func promptTranscript(ctx context.Context, d deps.Dependencies, rawDefault string, inline func() (string, error)) (string, error) {
	editor := resolveEditor(d)
	stdinFile, _ := d.Stdin.(*os.File)
	if editor == "" || d.Stdout == nil || !stdinIsTerminal(stdinFile) {
		return inline()
	}
	text, err := editTranscriptFn(ctx, editor, rawDefault)
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("corrected transcript text must not be empty")
	}
	return text, nil
}

// editTranscriptInEditor writes text to a private temp file, runs editor on it
// attached to the terminal, and returns the edited text without comment lines.
func editTranscriptInEditor(ctx context.Context, editor, text string) (string, error) {
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty editor command")
	}
	dir, err := os.MkdirTemp("", "voxi-transcript-")
	if err != nil {
		return "", fmt.Errorf("create transcript temp dir: %w", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "transcript.txt")
	body := "# Type exactly what you said, word for word. Lines starting with # are ignored.\n" + text + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", fmt.Errorf("write transcript temp file: %w", err)
	}
	cmd := exec.CommandContext(ctx, fields[0], append(fields[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run editor %q: %w", editor, err)
	}
	edited, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read edited transcript: %w", err)
	}
	return stripCommentLines(string(edited)), nil
}

// stripCommentLines drops lines starting with '#' and flattens the rest into
// one sanitized line, as corpus.tsv requires.
func stripCommentLines(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return sanitizeText(strings.Join(kept, "\n"))
}
