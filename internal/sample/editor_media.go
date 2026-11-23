package sample

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"ubunatic.com/loom"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/devsample"
	"ubunatic.com/voxi/spec"
)

// chunkSource is the subset of *chunks.Buffer needed to import recent chunks.
type chunkSource interface {
	List(reverse bool) ([]chunks.Chunk, error)
	ReadWAV(c chunks.Chunk) ([]byte, error)
}

func defaultSamplePlayer(d deps.Dependencies) func(ctx context.Context, wavPath string) error {
	return func(ctx context.Context, wavPath string) error {
		name, args, err := devsample.PlayerCommand(d)
		if err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, name, args(wavPath)...)
		return cmd.Run()
	}
}

// defaultSampleRecorder drives devsample.CaptureUtteranceWithReader via a controlled
// io.Pipe so it never contends with loom.Pane's raw terminal stdin.
func defaultSampleRecorder(d deps.Dependencies) func(ctx context.Context, stop <-chan struct{}) ([]byte, time.Duration, error) {
	return func(ctx context.Context, stop <-chan struct{}) ([]byte, time.Duration, error) {
		pr, pw := io.Pipe()
		defer pr.Close()
		go func() {
			select {
			case <-stop:
				_, _ = pw.Write([]byte("\n"))
			case <-ctx.Done():
			}
			_ = pw.Close()
		}()
		return devsample.CaptureUtteranceWithReader(ctx, d, bufio.NewReader(pr))
	}
}

func defaultSampleTranscriber(d deps.Dependencies) func(ctx context.Context, pcm []byte) (string, error) {
	return func(ctx context.Context, pcm []byte) (string, error) {
		return devsample.TranscribeRawTranscript(ctx, d, pcm)
	}
}

func defaultKeytermSuggester(d deps.Dependencies) func(text string) []string {
	return func(text string) []string {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		modelSpec, err := spec.LoadModels()
		if err != nil {
			return nil
		}
		home := ""
		if d.Getenv != nil {
			home = d.Getenv("HOME")
		}
		return devsample.SuggestKeyterms(text, devsample.CandidateVocabulary(home, modelSpec), modelSpec.SpeechContext.MaxTermChars)
	}
}

func defaultChunkSource(d deps.Dependencies) chunkSource {
	if d.Getenv != nil {
		return chunks.NewBuffer(chunks.StorageDir(d.Getenv("XDG_RUNTIME_DIR"), d.Getenv("HOME")), chunks.DefaultBufferSize)
	}
	return chunks.DefaultBuffer()
}

// cleanup cancels any active background playback or recording and removes temp WAV files.
func (e *sampleEditor) cleanup() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cleanupLocked()
}

func (e *sampleEditor) cleanupLocked() {
	e.stopPlaybackLocked()
	e.cancelRecordingLocked()
	e.removeTempWAVLocked()
}

func (e *sampleEditor) removeTempWAVLocked() {
	if e.addWAVPath != "" {
		_ = os.Remove(e.addWAVPath)
		e.addWAVPath = ""
	}
}

func (e *sampleEditor) stopPlaybackLocked() {
	if e.playCancel != nil {
		e.playCancel()
		e.playCancel = nil
		e.playID = ""
		e.playSeq++
	}
}

func (e *sampleEditor) togglePlaybackLocked() {
	if e.playCancel != nil {
		e.stopPlaybackLocked()
		e.setStatus("playback stopped")
		return
	}
	x, ok := e.selected()
	if !ok {
		e.setStatus("no sample selected")
		return
	}
	if x.Audio == "" {
		e.setStatus(x.ID + " has no audio")
		return
	}
	wavPath := e.store.AudioPath(x)
	if _, err := os.Stat(wavPath); err != nil {
		e.setStatus("audio file missing for " + x.ID)
		return
	}
	if e.play == nil {
		e.setStatus("audio playback unavailable")
		return
	}
	pctx, cancel := context.WithCancel(e.ctx)
	e.playCancel = cancel
	e.playID = x.ID
	e.playSeq++
	seq := e.playSeq
	playFn := e.play
	e.setStatus("playing " + x.ID + "… (p/Space to stop)")
	e.queueLocked(func() {
		err := playFn(pctx, wavPath)
		e.mu.Lock()
		if e.playSeq == seq {
			e.playCancel = nil
			e.playID = ""
			if err != nil && pctx.Err() == nil {
				e.setStatus("playback failed: " + err.Error())
			} else if pctx.Err() == nil {
				e.setStatus("finished playing " + x.ID)
			}
		}
		inv := e.invalidate
		e.mu.Unlock()
		if inv != nil {
			inv()
		}
	})
}

func (e *sampleEditor) beginAddLocked() {
	e.mode = modeAddChoice
	e.setStatus("Add sample: [1] Record from mic · [2] Import from recent chunks · [Esc] Cancel")
}

func (e *sampleEditor) addChoiceKey(k loom.KeyEvent) {
	switch k.Name() {
	case "esc":
		e.mode = modeBrowse
		e.setStatus("add cancelled")
	case "1":
		e.startMicRecordingLocked()
	case "2":
		e.openChunkPickerLocked()
	}
}

func (e *sampleEditor) cancelRecordingLocked() {
	if e.recCancel != nil {
		e.recCancel()
		e.recCancel = nil
	}
	if e.recStop != nil {
		e.recStopOnce.Do(func() { close(e.recStop) })
		e.recStop = nil
	}
	e.recSeq++
}

func (e *sampleEditor) startMicRecordingLocked() {
	if e.record == nil {
		e.mode = modeBrowse
		e.setStatus("microphone recording unavailable")
		return
	}
	e.cancelRecordingLocked()
	rctx, cancel := context.WithCancel(e.ctx)
	stopCh := make(chan struct{})
	e.recCancel = cancel
	e.recStop = stopCh
	e.recStopOnce = sync.Once{}
	e.recSeq++
	seq := e.recSeq
	e.recStarted = e.now()
	e.mode = modeRecording
	e.setStatus("Recording... Enter/Space stop · Esc cancel")

	recFn := e.record
	trFn := e.transcribe
	sugFn := e.suggestKeyterms
	e.queueLocked(func() {
		pcm, dur, err := recFn(rctx, stopCh)
		if rctx.Err() != nil {
			return
		}
		if err != nil {
			e.mu.Lock()
			if e.recSeq == seq {
				e.recCancel = nil
				e.recStop = nil
				e.mode = modeBrowse
				e.setStatus("recording failed: " + err.Error())
			}
			inv := e.invalidate
			e.mu.Unlock()
			if inv != nil {
				inv()
			}
			return
		}
		tmpPath, wavErr := writeTempWAV(pcm)
		if wavErr != nil {
			e.mu.Lock()
			if e.recSeq == seq {
				e.recCancel = nil
				e.recStop = nil
				e.mode = modeBrowse
				e.setStatus("save temp wav failed: " + wavErr.Error())
			}
			inv := e.invalidate
			e.mu.Unlock()
			if inv != nil {
				inv()
			}
			return
		}
		raw := ""
		statusNote := fmt.Sprintf("captured %.1fs; fill ID and press Enter to save", dur.Seconds())
		if trFn != nil {
			if tr, trErr := trFn(rctx, pcm); trErr != nil {
				if rctx.Err() != nil {
					_ = os.Remove(tmpPath)
					return
				}
				statusNote = fmt.Sprintf("captured %.1fs (ASR unavailable: %v)", dur.Seconds(), trErr)
			} else {
				raw = strings.TrimSpace(tr)
			}
		}
		var suggested []string
		if sugFn != nil && raw != "" {
			suggested = sugFn(raw)
		}
		e.mu.Lock()
		if e.recSeq != seq {
			e.mu.Unlock()
			_ = os.Remove(tmpPath)
			return
		}
		e.recCancel = nil
		e.recStop = nil
		e.openAddFormLocked("record", tmpPath, raw, suggested, statusNote)
		inv := e.invalidate
		e.mu.Unlock()
		if inv != nil {
			inv()
		}
	})
}

func (e *sampleEditor) recordingKey(k loom.KeyEvent) {
	switch k.Name() {
	case "enter", " ":
		if e.recStop != nil {
			e.recStopOnce.Do(func() { close(e.recStop) })
			e.setStatus("Stopping recording & transcribing…")
		}
	case "esc":
		e.cancelRecordingLocked()
		e.mode = modeBrowse
		e.setStatus("recording cancelled")
	}
}

func (e *sampleEditor) openChunkPickerLocked() {
	if e.chunkBuf == nil {
		e.mode = modeBrowse
		e.setStatus("no recent chunks available")
		return
	}
	list, err := e.chunkBuf.List(true)
	if err != nil {
		e.mode = modeBrowse
		e.setStatus("recent chunks unavailable: " + err.Error())
		return
	}
	if len(list) == 0 {
		e.mode = modeBrowse
		e.setStatus("no recent chunks available")
		return
	}
	e.recentChunks = list
	e.chunkCursor = 0
	e.mode = modePickChunk
	e.setStatus(fmt.Sprintf("Select a recent chunk (%d available)", len(list)))
}

func (e *sampleEditor) pickChunkKey(k loom.KeyEvent) {
	switch k.Name() {
	case "esc":
		e.recentChunks = nil
		e.mode = modeBrowse
		e.setStatus("import cancelled")
	case "j", "down":
		if e.chunkCursor < len(e.recentChunks)-1 {
			e.chunkCursor++
		}
	case "k", "up":
		if e.chunkCursor > 0 {
			e.chunkCursor--
		}
	case "enter":
		if e.chunkCursor < 0 || e.chunkCursor >= len(e.recentChunks) {
			return
		}
		ch := e.recentChunks[e.chunkCursor]
		wav, err := e.chunkBuf.ReadWAV(ch)
		if err != nil {
			e.setStatus("read chunk audio failed: " + err.Error())
			return
		}
		tmpPath, err := writeTempRawWAV(wav)
		if err != nil {
			e.setStatus("write temp chunk wav failed: " + err.Error())
			return
		}
		text := strings.TrimSpace(ch.CleanedTranscript)
		if text == "" {
			text = strings.TrimSpace(ch.RawTranscript)
		}
		var suggested []string
		if e.suggestKeyterms != nil && text != "" {
			suggested = e.suggestKeyterms(text)
		}
		source := fmt.Sprintf("chunk:%s/%d", ch.SessionID, ch.Index)
		e.recentChunks = nil
		e.openAddFormLocked(source, tmpPath, text, suggested, "imported "+source+"; fill ID and press Enter to save")
	}
}

func (e *sampleEditor) openAddFormLocked(source, wavPath, transcript string, keyterms []string, status string) {
	e.removeTempWAVLocked()
	e.mode = modeAddForm
	e.addSource = source
	e.addWAVPath = wavPath
	e.addPurpose = Dictation
	e.addID = loom.NewTextInput("")
	e.addTranscript = loom.NewTextInput(transcript)
	e.addKeyterms = loom.NewTextInput(strings.Join(NormalizeKeyterms(keyterms), ", "))
	e.addField = 0
	e.setStatus(status)
}

func (e *sampleEditor) addFormKey(k loom.KeyEvent) {
	switch k.Name() {
	case "esc":
		e.removeTempWAVLocked()
		e.mode = modeBrowse
		e.setStatus("add cancelled")
	case "tab":
		e.addField = (e.addField + 1) % 4
	case "shift-tab":
		e.addField = (e.addField + 3) % 4
	case "enter":
		e.submitAddFormLocked()
	default:
		switch e.addField {
		case 0:
			e.addID.ConsumeKey(k)
		case 1:
			e.addPurposeKey(k)
		case 2:
			e.addTranscript.ConsumeKey(k)
		case 3:
			e.addKeyterms.ConsumeKey(k)
		}
	}
}

func (e *sampleEditor) addPurposeKey(k loom.KeyEvent) {
	order := []Purpose{Dictation, Noise, Voice}
	switch k.Name() {
	case "d":
		e.addPurpose = Dictation
	case "n":
		e.addPurpose = Noise
	case "v":
		e.addPurpose = Voice
	case "right", " ":
		for i, p := range order {
			if p == e.addPurpose {
				e.addPurpose = order[(i+1)%len(order)]
				return
			}
		}
	case "left":
		for i, p := range order {
			if p == e.addPurpose {
				e.addPurpose = order[(i+len(order)-1)%len(order)]
				return
			}
		}
	}
}

func (e *sampleEditor) submitAddFormLocked() {
	id := strings.TrimSpace(e.addID.Value())
	if !validID(id) {
		e.setStatus("invalid sample id; use letters, digits, - or _")
		return
	}
	exists, err := e.store.Has(id)
	if err != nil {
		e.setStatus("check sample id failed: " + err.Error())
		return
	}
	if exists {
		e.setStatus(fmt.Sprintf("sample %q already exists", id))
		return
	}
	text := strings.TrimSpace(e.addTranscript.Value())
	if requiresTranscript(e.addPurpose) && text == "" {
		e.setStatus(fmt.Sprintf("%s samples need a transcript; type one, or Esc to cancel", e.addPurpose))
		return
	}
	if e.addPurpose == Voice {
		e.mode = modeConfirmAddVoice
		e.setStatus("This is my own voice and I consent to cloning it? [y/N]")
		return
	}
	e.commitAddLocked(nil)
}

func (e *sampleEditor) confirmAddVoiceKey(k loom.KeyEvent) {
	switch k.Name() {
	case "y", "Y":
		now := e.now().UTC()
		e.commitAddLocked(&now)
	case "n", "N", "esc", "enter":
		e.mode = modeAddForm
		e.setStatus("voice consent declined; choose another purpose or Esc to cancel")
	}
}

func (e *sampleEditor) commitAddLocked(consent *time.Time) {
	id := strings.TrimSpace(e.addID.Value())
	text := strings.TrimSpace(e.addTranscript.Value())
	keyterms := NormalizeKeyterms(strings.Split(e.addKeyterms.Value(), ","))
	x := Sample{
		ID:         id,
		Purpose:    e.addPurpose,
		Transcript: text,
		Keyterms:   keyterms,
		Created:    e.now(),
		Source:     e.addSource,
		Consent:    consent,
	}
	if err := e.store.Put(x, e.addWAVPath); err != nil {
		e.mode = modeAddForm
		e.setStatus("add failed: " + err.Error())
		return
	}
	e.removeTempWAVLocked()
	e.mode = modeBrowse
	e.reload(id)
	e.setStatus("added " + id)
}

func (e *sampleEditor) drawAddMode(c *loom.Canvas, r loom.Rect) bool {
	switch e.mode {
	case modeAddChoice:
		writeLine(c, r, 0, "Add Sample", loom.Style{Bold: true})
		writeLine(c, r, 2, "[1] Record from mic", loom.Style{})
		writeLine(c, r, 3, "[2] Import from recent chunks", loom.Style{})
		writeLine(c, r, 5, "[Esc] Cancel", loom.Style{Dim: true})
		return true
	case modeRecording:
		writeLine(c, r, 0, "Recording from microphone…", loom.Style{Bold: true})
		writeLine(c, r, 2, "Speak now. Press Enter or Space to stop.", loom.Style{})
		writeLine(c, r, 3, "Press Esc to cancel.", loom.Style{Dim: true})
		return true
	case modePickChunk:
		writeLine(c, r, 0, "Import from recent chunks:", loom.Style{Bold: true})
		bodyH := r.H - 1
		if bodyH <= 0 {
			return true
		}
		start := 0
		if e.chunkCursor >= bodyH {
			start = e.chunkCursor - bodyH + 1
		}
		for i := 0; i < bodyH && start+i < len(e.recentChunks); i++ {
			idx := start + i
			ch := e.recentChunks[idx]
			mark := "  "
			if idx == e.chunkCursor {
				mark = "> "
			}
			preview := strings.TrimSpace(ch.CleanedTranscript)
			if preview == "" {
				preview = strings.TrimSpace(ch.RawTranscript)
			}
			if preview == "" {
				preview = "(no transcript)"
			}
			line := fmt.Sprintf("%s#%d %.1fs  %s", mark, ch.Index, ch.AudioDurationSecs, preview)
			writeLine(c, r, i+1, line, loom.Style{Bold: idx == e.chunkCursor})
		}
		return true
	case modeAddForm, modeConfirmAddVoice:
		e.drawAddForm(c, r)
		return true
	}
	return false
}

func (e *sampleEditor) drawAddForm(c *loom.Canvas, r loom.Rect) {
	y := r.Y
	end := r.Y + r.H
	label := func(text string) {
		if y < end {
			c.Write(r.X, y, clip(text, r.W), loom.Style{Bold: true})
		}
		y++
	}
	input := func(in *loom.TextInput, focused bool) {
		if y < end && in != nil {
			in.Draw(c, loom.Rect{X: r.X, Y: y, W: r.W, H: 1}, focused)
		}
		y++
	}
	label("New sample (" + e.addSource + ")")
	label("ID:")
	input(e.addID, e.mode == modeAddForm && e.addField == 0)
	purposeStyle := loom.Style{}
	if e.mode == modeAddForm && e.addField == 1 {
		purposeStyle = loom.Style{Bold: true, Underline: true}
	}
	if y < end {
		c.Write(r.X, y, clip("Purpose: ["+string(e.addPurpose)+"] (d/n/v)", r.W), purposeStyle)
	}
	y++
	label("Transcript:")
	input(e.addTranscript, e.mode == modeAddForm && e.addField == 2)
	label("Keyterms, comma-separated:")
	input(e.addKeyterms, e.mode == modeAddForm && e.addField == 3)
}

func writeTempWAV(pcm []byte) (string, error) {
	f, err := os.CreateTemp("", "voxi-sample-rec-*.wav")
	if err != nil {
		return "", err
	}
	path := f.Name()
	_ = f.Close()
	if err := audio.WriteWAVAudio(path, pcm, devsample.SampleRate); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func writeTempRawWAV(wav []byte) (string, error) {
	f, err := os.CreateTemp("", "voxi-sample-chunk-*.wav")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.Write(wav); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}
