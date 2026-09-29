package devsample

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"ubunatic.com/voxi/internal/deps"
)

func sanitizeText(text string) string {
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.TrimSpace(text)
}

// promptText reads the manually corrected ground-truth transcript from
// stdin: what the user actually said, typed/pasted exactly, not raw ASR
// output. rawDefault, when non-empty, is small.en's raw unprompted ASR
// guess for the just-captured audio (see transcribeRawTranscript).
//
// When stdinFile is a real terminal, this runs the raw-mode line editor
// (lineedit.go): the prompt line starts pre-filled with rawDefault, cursor
// at the end, cursor-navigable and in-place editable (see editLine's doc
// comment for the supported keys). Enter submits the current content
// unmodified if untouched — matching "unedited Enter accepts rawDefault
// verbatim" exactly, just via real in-place editing rather than
// accept-or-retype. Ctrl-C returns errAborted, aborting the whole Record
// call (no disk write has happened yet at this point in Record).
//
// Otherwise (piped stdin, a non-terminal *bufio.Reader as in tests, or
// stdin isn't a terminal at all) this falls back to the pre-045 behavior:
// print rawDefault as a visible suggestion, then read one plain line; a
// blank Enter accepts rawDefault verbatim, anything typed replaces it
// wholesale.
//
// in must be the single shared *bufio.Reader used for every stdin prompt in
// this Record call (see captureUtteranceWithReader's doc comment on why).
func promptText(out io.Writer, in *bufio.Reader, stdinFile *os.File, prompt, rawDefault string) (string, error) {
	if in == nil {
		if out != nil {
			if rawDefault != "" {
				fmt.Fprintf(out, "Raw ASR guess: %q\n", rawDefault)
			}
			fmt.Fprint(out, prompt)
		}
		if rawDefault != "" {
			return rawDefault, nil
		}
		return "", fmt.Errorf("no input available to read the corrected transcript")
	}

	if useLineEditor(out, stdinFile) {
		line, err := editLine(out, in, stdinFile, prompt, rawDefault)
		if err != nil {
			return "", err
		}
		line = sanitizeText(line)
		if line == "" {
			if rawDefault != "" {
				return rawDefault, nil
			}
			return "", fmt.Errorf("corrected transcript text must not be empty")
		}
		return line, nil
	}

	if out != nil {
		if rawDefault != "" {
			fmt.Fprintf(out, "Raw ASR guess: %q\n", rawDefault)
		}
		fmt.Fprint(out, prompt)
	}
	line, err := in.ReadString('\n')
	if err != nil && !(err == io.EOF && line != "") {
		if err == io.EOF {
			if rawDefault != "" {
				return rawDefault, nil
			}
			return "", fmt.Errorf("no corrected transcript entered")
		}
		return "", fmt.Errorf("read corrected transcript: %w", err)
	}
	line = sanitizeText(line)
	if line == "" {
		if rawDefault != "" {
			return rawDefault, nil
		}
		return "", fmt.Errorf("corrected transcript text must not be empty")
	}
	return line, nil
}

// PromptText exposes the transcript prompt for commands that share this flow.
func PromptText(out io.Writer, in *bufio.Reader, stdinFile *os.File, prompt, rawDefault string) (string, error) {
	return promptText(out, in, stdinFile, prompt, rawDefault)
}

// promptKeyterms reads comma- or `|`-separated keyterms.
// suggested is the pre-computed intersection of the corrected transcript
// against the known vocabulary (see suggestKeyterms); a blank Enter (or
// EOF, or no input reader) accepts it as-is, which is empty whenever
// nothing matched — the same empty-keyterms outcome as before this ticket.
//
// Like promptText, this runs the raw-mode line editor (pre-filled with
// suggested, cursor at the end) when stdinFile is a real terminal, falling
// back to a plain accept-or-retype read otherwise. Ctrl-C returns
// errAborted; unlike a plain read error, callers must check for it
// explicitly and propagate it rather than treating it as "keyterms
// unavailable, leave them empty" (see Record).
func promptKeyterms(out io.Writer, in *bufio.Reader, stdinFile *os.File, suggested string) (string, error) {
	prompt := "Enter keyterms (comma or | separated, optional; Enter for none): "
	if suggested != "" {
		prompt = "Press Enter to accept, or type replacement keyterms (comma or | separated): "
	}

	if in == nil {
		if out != nil {
			if suggested != "" {
				fmt.Fprintf(out, "Suggested keyterms: %s\n", suggested)
			}
			fmt.Fprint(out, prompt)
		}
		return suggested, nil
	}

	if useLineEditor(out, stdinFile) {
		line, err := editLine(out, in, stdinFile, prompt, suggested)
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return suggested, nil
		}
		return normalizeKeyterms(line), nil
	}

	if out != nil {
		if suggested != "" {
			fmt.Fprintf(out, "Suggested keyterms: %s\n", suggested)
		}
		fmt.Fprint(out, prompt)
	}
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read keyterms: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return suggested, nil
	}
	return normalizeKeyterms(line), nil
}

// PromptKeyterms exposes the shared keyterm prompt for sample commands.
func PromptKeyterms(out io.Writer, in *bufio.Reader, stdinFile *os.File, suggested string) (string, error) {
	return promptKeyterms(out, in, stdinFile, suggested)
}

// PlayerCommand picks a standard local audio player, mirroring the same
// LookPath-fallback style used to pick the capture tool.
func PlayerCommand(d deps.Dependencies) (name string, args func(wavPath string) []string, err error) {
	for _, candidate := range []struct {
		name string
		args func(string) []string
	}{
		{"paplay", func(p string) []string { return []string{p} }},
		{"aplay", func(p string) []string { return []string{p} }},
		{"ffplay", func(p string) []string { return []string{"-nodisp", "-autoexit", "-loglevel", "quiet", p} }},
	} {
		if _, lookErr := d.LookPath(candidate.name); lookErr == nil {
			return candidate.name, candidate.args, nil
		}
	}
	return "", nil, fmt.Errorf("no local audio player found (looked for paplay, aplay, ffplay)")
}
