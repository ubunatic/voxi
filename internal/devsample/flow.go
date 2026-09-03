package devsample

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// captureFn is captureUtteranceWithReader by default; tests override it to
// avoid depending on a real microphone / pw-record / arecord being present.
var captureFn = captureUtteranceWithReader

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

// promptKeyterms reads the manifest's keyterms field: comma- or
// `|`-separated terms, matching corpus.tsv's `|` convention on output.
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

// confirm reads a yes/no answer from stdin, defaulting to no.
func confirm(out io.Writer, in *bufio.Reader, prompt string) bool {
	if out != nil {
		fmt.Fprint(out, prompt)
	}
	if in == nil {
		return false
	}
	line, _ := in.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// Record captures one utterance for name, prompts for its corrected
// transcript, and persists both atomically. force skips the overwrite
// confirmation for an existing name.
//
// Ordering keeps the on-disk state always either fully absent or fully
// complete for a given name: the microphone capture stays in memory, the
// corrected text is confirmed non-empty, the WAV is written to a temp file
// and only then renamed into place, and the manifest (which is itself
// written via temp file + rename) is updated last. A crash before the final
// manifest write leaves, at worst, an unreferenced orphan WAV — never a
// half-written or truncated one.
func Record(ctx context.Context, d deps.Dependencies, home, rawName string, force bool) error {
	name, err := SanitizeName(rawName)
	if err != nil {
		return err
	}

	// One shared *bufio.Reader over d.Stdin for the whole flow (overwrite
	// confirmation, the recorder's stop-on-Enter trigger, and the transcript
	// prompt): a fresh bufio.Reader per prompt would silently drop whatever
	// the previous one had already buffered ahead from the same stream.
	var in *bufio.Reader
	if d.Stdin != nil {
		in = bufio.NewReader(d.Stdin)
	}
	// stdinFile is non-nil only when d.Stdin is a real *os.File (production
	// stdin), letting promptText/promptKeyterms opt into raw-mode line
	// editing; it stays nil for piped input and every test's
	// strings.Reader/bytes.Reader stdin, which fall back unchanged.
	stdinFile, _ := d.Stdin.(*os.File)

	samples, err := LoadManifest(home)
	if err != nil {
		return err
	}
	if _, exists := Find(samples, name); exists && !force {
		if !confirm(d.Stdout, in, fmt.Sprintf("Sample %q already exists. Overwrite? [y/N] ", name)) {
			return fmt.Errorf("aborted: sample %q already exists", name)
		}
	}

	pcm, duration, err := captureFn(ctx, d, in)
	if err != nil {
		return err
	}
	if d.Stdout != nil {
		fmt.Fprintf(d.Stdout, "Captured %.1fs of audio.\n", duration.Seconds())
	}

	rawTranscript := ""
	if raw, terr := transcribeFn(ctx, d, pcm); terr != nil {
		if d.Stdout != nil {
			fmt.Fprintf(d.Stdout, "Note: raw ASR transcript unavailable (%v); starting from a blank transcript.\n", terr)
		}
	} else {
		rawTranscript = raw
	}

	text, err := promptText(d.Stdout, in, stdinFile, "Enter the corrected transcript (what you actually said): ", rawTranscript)
	if err != nil {
		return err
	}

	keyterms := ""
	if modelSpec, specErr := spec.LoadModels(); specErr != nil {
		if d.Stdout != nil {
			fmt.Fprintf(d.Stdout, "Note: keyterm suggestions unavailable (%v); leaving keyterms empty.\n", specErr)
		}
	} else {
		suggested := strings.Join(suggestKeyterms(text, candidateVocabulary(home, modelSpec), modelSpec.SpeechContext.MaxTermChars), "|")
		kt, kerr := promptKeyterms(d.Stdout, in, stdinFile, suggested)
		if kerr != nil {
			if errors.Is(kerr, errAborted) {
				return kerr
			}
			if d.Stdout != nil {
				fmt.Fprintf(d.Stdout, "Note: keyterm prompt failed (%v); leaving keyterms empty.\n", kerr)
			}
		} else {
			keyterms = kt
		}
	}

	dir := SamplesDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create samples directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure samples directory: %w", err)
	}

	wavPath := WAVPath(home, name)
	tmpWAVPath := wavPath + ".tmp"
	defer os.Remove(tmpWAVPath)
	if err := audio.WriteWAVAudio(tmpWAVPath, pcm, SampleRate); err != nil {
		return fmt.Errorf("write sample audio: %w", err)
	}
	if err := os.Rename(tmpWAVPath, wavPath); err != nil {
		return fmt.Errorf("finalize sample audio: %w", err)
	}
	if err := os.Chmod(wavPath, 0o600); err != nil {
		return fmt.Errorf("secure sample audio: %w", err)
	}

	samples = Upsert(samples, Sample{
		Name:      name,
		WAVFile:   name + ".wav",
		Text:      text,
		Keyterms:  keyterms,
		Timestamp: time.Now(),
	})
	if err := SaveManifest(home, samples); err != nil {
		return fmt.Errorf("save sample manifest (audio saved at %s): %w", wavPath, err)
	}

	if d.Stdout != nil {
		fmt.Fprintf(d.Stdout, "Saved sample %q (%s)\n", name, wavPath)
	}
	return nil
}

// Remove deletes both the WAV and its manifest entry for name.
func Remove(home, rawName string) error {
	name, err := SanitizeName(rawName)
	if err != nil {
		return err
	}
	samples, err := LoadManifest(home)
	if err != nil {
		return err
	}
	if _, exists := Find(samples, name); !exists {
		return fmt.Errorf("sample %q was not found", name)
	}
	wavPath := WAVPath(home, name)
	if err := os.Remove(wavPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove sample audio: %w", err)
	}
	samples, _ = RemoveEntry(samples, name)
	if err := SaveManifest(home, samples); err != nil {
		return fmt.Errorf("update sample manifest: %w", err)
	}
	return nil
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

// Play re-plays the stored WAV for name through a standard local player.
func Play(ctx context.Context, d deps.Dependencies, home, rawName string) error {
	name, err := SanitizeName(rawName)
	if err != nil {
		return err
	}
	samples, err := LoadManifest(home)
	if err != nil {
		return err
	}
	if _, exists := Find(samples, name); !exists {
		return fmt.Errorf("sample %q was not found", name)
	}
	wavPath := WAVPath(home, name)
	if _, statErr := os.Stat(wavPath); statErr != nil {
		return fmt.Errorf("sample audio missing: %w", statErr)
	}
	playerName, playerArgs, err := PlayerCommand(d)
	if err != nil {
		return err
	}
	if d.Run == nil {
		return fmt.Errorf("no runner available to play %s", wavPath)
	}
	if err := d.Run(ctx, playerName, playerArgs(wavPath)...); err != nil {
		return fmt.Errorf("play sample with %s: %w", playerName, err)
	}
	return nil
}
