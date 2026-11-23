package sample

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"ubunatic.com/loom"
)

var fixedNow = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// newTestEditor opens a store in t.TempDir, adds xs, and returns an editor with a fixed clock.
func newTestEditor(t *testing.T, xs ...Sample) (*sampleEditor, *Store) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "samples"))
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range xs {
		if err := s.Add(x); err != nil {
			t.Fatalf("add %s: %v", x.ID, err)
		}
	}
	e := newSampleEditor(s)
	e.now = func() time.Time { return fixedNow }
	return e, s
}

// press sends raw terminal bytes through the same decoder the pane uses.
func press(e *sampleEditor, raw string) loom.EventResult {
	return e.ConsumeKey(loom.DecodeKey([]byte(raw)))
}

func pressKeys(e *sampleEditor, raws ...string) {
	for _, raw := range raws {
		press(e, raw)
	}
}

func typeText(e *sampleEditor, text string) {
	for _, r := range text {
		press(e, string(r))
	}
}

// clearField sends enough backspaces to empty the focused input.
func clearField(e *sampleEditor) {
	for i := 0; i < 200; i++ {
		press(e, "\x7f")
	}
}

// screen renders the editor and strips ANSI so tests can match plain text.
func screen(e *sampleEditor, cols, rows int) []string {
	raw := loom.Render(e, cols, rows)
	out := make([]string, len(raw))
	for i, line := range raw {
		out[i] = ansiPattern.ReplaceAllString(line, "")
	}
	return out
}

func screenText(e *sampleEditor, cols, rows int) string {
	return strings.Join(screen(e, cols, rows), "\n")
}

func visibleIDs(e *sampleEditor) string {
	var ids []string
	for _, x := range e.visible() {
		ids = append(ids, x.ID)
	}
	return strings.Join(ids, ",")
}

func storedPurpose(t *testing.T, s *Store, id string) Purpose {
	t.Helper()
	x, err := s.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return x.Purpose
}

func TestEditorTabsFilterByPurpose(t *testing.T) {
	e, _ := newTestEditor(t,
		Sample{ID: "d1", Purpose: Dictation, Transcript: "one"},
		Sample{ID: "n1", Purpose: Noise},
		Sample{ID: "v1", Purpose: Voice, Transcript: "me", Consent: &fixedNow},
	)
	if got := visibleIDs(e); got != "d1,n1,v1" {
		t.Fatalf("All tab = %q", got)
	}
	for _, tc := range []struct{ key, want string }{
		{"2", "d1"}, {"3", "n1"}, {"4", "v1"}, {"1", "d1,n1,v1"},
	} {
		press(e, tc.key)
		if got := visibleIDs(e); got != tc.want {
			t.Fatalf("key %s: visible = %q, want %q", tc.key, got, tc.want)
		}
	}
	// Tab cycles All -> dictation -> noise -> voice -> All; shift-tab goes back.
	pressKeys(e, "tab")
	if got := visibleIDs(e); got != "d1" {
		t.Fatalf("tab from All = %q", got)
	}
	pressKeys(e, "tab", "tab", "tab")
	if got := visibleIDs(e); got != "d1,n1,v1" {
		t.Fatalf("tab wrap = %q", got)
	}
	pressKeys(e, "shift-tab")
	if got := visibleIDs(e); got != "v1" {
		t.Fatalf("shift-tab = %q", got)
	}
	if out := screenText(e, 80, 14); !strings.Contains(out, "[4] voice") {
		t.Fatalf("tab bar missing:\n%s", out)
	}
}

func TestEditorSearchFiltersAndEmptyMatches(t *testing.T) {
	e, _ := newTestEditor(t,
		Sample{ID: "kube", Purpose: Dictation, Transcript: "deploy the cluster", Keyterms: []string{"kubectl"}},
		Sample{ID: "tea", Purpose: Dictation, Transcript: "make tea"},
		Sample{ID: "room", Purpose: Noise},
	)
	press(e, "/")
	typeText(e, "KUBECTL")
	if got := visibleIDs(e); got != "kube" {
		t.Fatalf("keyterm search = %q", got)
	}
	press(e, "\r")
	if e.mode != modeBrowse || e.query != "KUBECTL" || visibleIDs(e) != "kube" {
		t.Fatalf("enter should keep the filter: mode=%v query=%q", e.mode, e.query)
	}
	if out := screenText(e, 80, 14); !strings.Contains(out, "(1 shown)") {
		t.Fatalf("filter count missing:\n%s", out)
	}
	press(e, "/")
	press(e, "\x1b")
	if e.query != "" || visibleIDs(e) != "kube,room,tea" {
		t.Fatalf("esc in search should clear: query=%q visible=%q", e.query, visibleIDs(e))
	}

	press(e, "/")
	typeText(e, "zzz")
	press(e, "\r")
	if got := visibleIDs(e); got != "" {
		t.Fatalf("zero-match search visible = %q", got)
	}
	if out := screenText(e, 80, 14); !strings.Contains(out, `No matches for "zzz"`) {
		t.Fatalf("empty-match message missing:\n%s", out)
	}
	pressKeys(e, "j", "k", "d", "y", "m", "n")
	if _, ok := e.selected(); ok {
		t.Fatal("selection exists with zero matches")
	}
	press(e, "e")
	if e.status != "no sample selected" {
		t.Fatalf("edit with no match status = %q", e.status)
	}
	pressKeys(e, "esc")
	if e.query != "" {
		t.Fatalf("esc in browse should clear the filter, query=%q", e.query)
	}
}

func TestEditorSearchTypingDoesNotQuit(t *testing.T) {
	e, _ := newTestEditor(t, Sample{ID: "quit-me", Purpose: Dictation, Transcript: "text"})
	press(e, "/")
	if res := press(e, "q"); res.Quit {
		t.Fatal("q typed in search must not quit")
	}
	if e.query != "q" {
		t.Fatalf("query = %q", e.query)
	}
	if res := press(e, "\x03"); !res.Quit {
		t.Fatal("ctrl-c in search must quit")
	}
}

func TestEditorEditTranscriptAndKeyterms(t *testing.T) {
	e, s := newTestEditor(t, Sample{ID: "greet", Purpose: Dictation, Transcript: "hello wrold"})
	press(e, "e")
	if e.mode != modeEdit {
		t.Fatalf("e did not open edit, mode=%v", e.mode)
	}
	clearField(e)
	typeText(e, "hello world")
	pressKeys(e, "tab")
	if e.editField != 1 {
		t.Fatalf("tab did not switch to keyterms, field=%d", e.editField)
	}
	typeText(e, "kubectl, kubectl ,, voxi")
	press(e, "\r")
	if e.mode != modeBrowse {
		t.Fatalf("enter should save and leave edit, mode=%v", e.mode)
	}
	x, err := s.Get("greet")
	if err != nil {
		t.Fatal(err)
	}
	if x.Transcript != "hello world" || strings.Join(x.Keyterms, "|") != "kubectl|voxi" {
		t.Fatalf("saved sample = %+v", x)
	}
	out := screenText(e, 80, 14)
	for _, want := range []string{"★", "kubectl, voxi", "Keyterms (jargon):"} {
		if !strings.Contains(out, want) {
			t.Fatalf("screen missing %q:\n%s", want, out)
		}
	}
}

func TestEditorRejectsEmptyTranscriptAndEscCancels(t *testing.T) {
	e, s := newTestEditor(t,
		Sample{ID: "keep", Purpose: Dictation, Transcript: "original"},
		Sample{ID: "quiet", Purpose: Noise},
	)
	press(e, "e")
	clearField(e)
	press(e, "\r")
	if e.mode != modeEdit || !strings.Contains(e.status, "need a transcript") {
		t.Fatalf("empty dictation save: mode=%v status=%q", e.mode, e.status)
	}
	if x, _ := s.Get("keep"); x.Transcript != "original" {
		t.Fatalf("rejected save changed transcript to %q", x.Transcript)
	}

	typeText(e, "discarded")
	press(e, "\x1b")
	if e.mode != modeBrowse || e.status != "edit cancelled" {
		t.Fatalf("esc: mode=%v status=%q", e.mode, e.status)
	}
	if x, _ := s.Get("keep"); x.Transcript != "original" {
		t.Fatalf("esc cancel changed transcript to %q", x.Transcript)
	}

	press(e, "j")
	press(e, "e")
	clearField(e)
	press(e, "\r")
	x, err := s.Get("quiet")
	if err != nil || e.mode != modeBrowse || x.Transcript != "" {
		t.Fatalf("empty noise transcript should save: mode=%v sample=%+v err=%v", e.mode, x, err)
	}
}

func TestEditorMoveAcrossPurposes(t *testing.T) {
	t.Run("dictation to noise", func(t *testing.T) {
		e, s := newTestEditor(t, Sample{ID: "a", Purpose: Dictation, Transcript: "hello"})
		pressKeys(e, "m", "n")
		if got := storedPurpose(t, s, "a"); got != Noise {
			t.Fatalf("purpose = %s, want noise", got)
		}
		if e.status != "moved a to noise" {
			t.Fatalf("status = %q", e.status)
		}
	})
	t.Run("voice without transcript is refused", func(t *testing.T) {
		e, s := newTestEditor(t, Sample{ID: "quiet", Purpose: Noise})
		pressKeys(e, "m", "v")
		if got := storedPurpose(t, s, "quiet"); got != Noise {
			t.Fatalf("refused move changed purpose to %s", got)
		}
		if e.mode != modeBrowse || !strings.Contains(e.status, "no transcript") {
			t.Fatalf("refusal: mode=%v status=%q", e.mode, e.status)
		}
	})
	t.Run("declined consent leaves the sample alone", func(t *testing.T) {
		e, s := newTestEditor(t, Sample{ID: "a", Purpose: Dictation, Transcript: "hello"})
		pressKeys(e, "m", "v")
		if e.mode != modeConfirmMove {
			t.Fatalf("voice move did not ask for consent, mode=%v", e.mode)
		}
		pressKeys(e, "n")
		x, _ := s.Get("a")
		if x.Purpose != Dictation || x.Consent != nil || e.mode != modeBrowse {
			t.Fatalf("declined consent: sample=%+v mode=%v", x, e.mode)
		}
	})
	t.Run("esc also declines consent", func(t *testing.T) {
		e, s := newTestEditor(t, Sample{ID: "a", Purpose: Dictation, Transcript: "hello"})
		pressKeys(e, "m", "v", "\x1b")
		x, _ := s.Get("a")
		if x.Purpose != Dictation || x.Consent != nil {
			t.Fatalf("esc consent: sample=%+v", x)
		}
	})
	t.Run("confirmed consent grants and moves", func(t *testing.T) {
		e, s := newTestEditor(t, Sample{ID: "a", Purpose: Dictation, Transcript: "hello"})
		pressKeys(e, "m", "v", "y")
		x, err := s.Get("a")
		if err != nil {
			t.Fatal(err)
		}
		if x.Purpose != Voice || x.Consent == nil || !x.Consent.Equal(fixedNow) {
			t.Fatalf("confirmed consent: sample=%+v", x)
		}
		if !strings.Contains(e.status, "moved a to voice") {
			t.Fatalf("status = %q", e.status)
		}
	})
	t.Run("voice back to noise needs no consent prompt", func(t *testing.T) {
		e, s := newTestEditor(t, Sample{ID: "v", Purpose: Voice, Transcript: "me", Consent: &fixedNow})
		pressKeys(e, "m", "n")
		if got := storedPurpose(t, s, "v"); got != Noise || e.mode != modeBrowse {
			t.Fatalf("voice to noise: purpose=%s mode=%v", got, e.mode)
		}
	})
	t.Run("same purpose is a no-op", func(t *testing.T) {
		e, s := newTestEditor(t, Sample{ID: "a", Purpose: Dictation, Transcript: "hello"})
		pressKeys(e, "m", "d")
		if got := storedPurpose(t, s, "a"); got != Dictation || e.status != "a is already in dictation" {
			t.Fatalf("same purpose: purpose=%s status=%q", got, e.status)
		}
	})
}

func TestEditorDeleteConfirmedAndCancelled(t *testing.T) {
	e, s := newTestEditor(t,
		Sample{ID: "a", Purpose: Dictation, Transcript: "one"},
		Sample{ID: "b", Purpose: Dictation, Transcript: "two"},
		Sample{ID: "c", Purpose: Dictation, Transcript: "three"},
	)
	pressKeys(e, "j", "j", "d", "n")
	if has, _ := s.Has("c"); !has || e.status != "delete cancelled" {
		t.Fatalf("declined delete: has=%v status=%q", has, e.status)
	}
	pressKeys(e, "d", "esc")
	if has, _ := s.Has("c"); !has {
		t.Fatal("esc removed the sample")
	}
	pressKeys(e, "d", "y")
	if has, _ := s.Has("c"); has {
		t.Fatal("confirmed delete kept the sample")
	}
	if got := visibleIDs(e); got != "a,b" || e.cursor != 1 {
		t.Fatalf("after delete: visible=%q cursor=%d", got, e.cursor)
	}

	press(e, "\x7f")
	pressKeys(e, "y")
	if got := visibleIDs(e); got != "a" || e.cursor != 0 {
		t.Fatalf("backspace delete: visible=%q cursor=%d", got, e.cursor)
	}
	pressKeys(e, "d", "y")
	if got := visibleIDs(e); got != "" || e.cursor != 0 {
		t.Fatalf("deleting the last sample: visible=%q cursor=%d", got, e.cursor)
	}
	if out := screenText(e, 80, 14); !strings.Contains(out, "No samples yet") {
		t.Fatalf("empty store message missing:\n%s", out)
	}
}

func TestEditorKeysQuitOnlyInBrowse(t *testing.T) {
	e, _ := newTestEditor(t, Sample{ID: "a", Purpose: Dictation, Transcript: "one"})
	if res := press(e, "\x1b"); res.Quit {
		t.Fatal("esc in browse quit the editor")
	}
	if res := press(e, "q"); !res.Quit {
		t.Fatal("q in browse did not quit")
	}
	if res := press(e, "\x03"); !res.Quit {
		t.Fatal("ctrl-c in browse did not quit")
	}
	press(e, "m")
	if res := press(e, "q"); res.Quit || e.mode != modeMove {
		t.Fatalf("q in move prompt: quit=%v mode=%v", res.Quit, e.mode)
	}
}

func TestEditorPlaybackMissingAudioSetsStatus(t *testing.T) {
	e, s := newTestEditor(t, Sample{ID: "a", Purpose: Dictation, Transcript: "one"})
	press(e, "p")
	if !strings.Contains(e.status, "no audio") {
		t.Fatalf("p status = %q", e.status)
	}
	if x, _ := s.Get("a"); x.Transcript != "one" {
		t.Fatalf("playback on missing audio changed the store: %+v", x)
	}
}

func TestEditorRowMarkersAndDetailPane(t *testing.T) {
	e, _ := newTestEditor(t,
		Sample{ID: "greet", Purpose: Dictation, Transcript: "hello there", Keyterms: []string{"voxi"}, Source: "record", Created: fixedNow},
		Sample{ID: "me", Purpose: Voice, Transcript: "my voice", Consent: &fixedNow},
	)
	out := screenText(e, 80, 14)
	for _, want := range []string{"[dict] ★", "ID: greet", "Purpose: dictation", "Source: record", "Transcript:", "hello there", "Keyterms (jargon):", "voxi"} {
		if !strings.Contains(out, want) {
			t.Fatalf("screen missing %q:\n%s", want, out)
		}
	}
	press(e, "4")
	out = screenText(e, 80, 14)
	if !strings.Contains(out, "[voic]  ✓") {
		t.Fatalf("voice row missing:\n%s", out)
	}
	if want := "Consent: given " + fixedNow.Local().Format("2006-01-02 15:04"); !strings.Contains(out, want) {
		t.Fatalf("consent detail missing:\n%s", out)
	}
}

func TestEditorNarrowAndUnicodeRenderSafely(t *testing.T) {
	long := "héllo 日本語 🎙 this transcript keeps going past any narrow pane width and wraps"
	e, _ := newTestEditor(t,
		Sample{ID: "wide", Purpose: Dictation, Transcript: long, Keyterms: []string{"日本語", "kubectl"}},
		Sample{ID: "empty", Purpose: Noise},
	)
	states := []struct {
		name string
		keys []string
	}{
		{"browse", nil},
		{"search", []string{"/", "h"}},
		{"edit", []string{"e"}},
		{"move", []string{"m"}},
		{"delete", []string{"d"}},
	}
	for _, st := range states {
		// Rendering does not change editor state, so one editor per mode serves every size.
		mode := newTestEditorCopy(e)
		pressKeys(mode, st.keys...)
		// Heights cover below the minimum, the minimum itself, the stacked split, and the design height.
		for cols := 1; cols <= 60; cols++ {
			for _, rows := range []int{0, 1, 4, 5, 6, 8, 10, 14, 16} {
				for _, line := range screen(mode, cols, rows) {
					if w := loom.StringWidth(line); w > cols {
						t.Fatalf("%s %dx%d: line width %d > %d: %q", st.name, cols, rows, w, cols, line)
					}
					if !utf8.ValidString(line) {
						t.Fatalf("%s %dx%d: invalid utf-8: %q", st.name, cols, rows, line)
					}
				}
			}
		}
	}

	press(e, "j") // IDs sort as "empty", "wide"; select the long transcript
	out := screenText(e, 40, 14)
	if !strings.Contains(out, "ID: wide") || !strings.Contains(out, "héllo") {
		t.Fatalf("40x14 detail missing:\n%s", out)
	}
	if small := screenText(e, 20, 3); !strings.Contains(small, "5 rows, have 3") {
		t.Fatalf("short pane message missing:\n%s", small)
	}
}

// newTestEditorCopy returns a fresh editor over the same store, so each render case starts in a known mode.
func newTestEditorCopy(e *sampleEditor) *sampleEditor {
	c := newSampleEditor(e.store)
	c.now = e.now
	return c
}

func TestEditorEmptyStoreRendersSafely(t *testing.T) {
	e, _ := newTestEditor(t)
	if out := screenText(e, 80, 14); !strings.Contains(out, "No samples yet") {
		t.Fatalf("empty store message missing:\n%s", out)
	}
	for _, cols := range []int{1, 40} {
		_ = screenText(e, cols, 14)
	}
	pressKeys(e, "e", "m", "d")
	if e.mode != modeBrowse || e.status != "no sample selected" {
		t.Fatalf("actions on empty store: mode=%v status=%q", e.mode, e.status)
	}
}

func TestWrapTextKeepsRunesWhole(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
	}{
		{"héllo 日本語 🎙 transcript", 5},
		{"日本語日本語", 1},
		{"日本語", 2},
		{"a verylongwordthatneedsbreaking", 4},
		{"", 3},
	} {
		lines := wrapText(tc.text, tc.width)
		for _, line := range lines {
			if w := loom.StringWidth(line); w > tc.width && !(tc.width == 1 && w == 2) {
				t.Errorf("wrapText(%q, %d): line %q width %d", tc.text, tc.width, line, w)
			}
			if !utf8.ValidString(line) {
				t.Errorf("wrapText(%q, %d): invalid utf-8 %q", tc.text, tc.width, line)
			}
		}
		if strings.Join(strings.Fields(strings.Join(lines, " ")), "") != strings.Join(strings.Fields(tc.text), "") {
			t.Errorf("wrapText(%q, %d) lost characters: %q", tc.text, tc.width, lines)
		}
	}
	if got := wrapText("anything", 0); got != nil {
		t.Fatalf("zero width = %q", got)
	}
}

func TestEditorLoomWidgetsWrapEditConfirmAndSplit(t *testing.T) {
	long := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma"
	e, s := newTestEditor(t, Sample{ID: "wrap", Purpose: Dictation, Transcript: long, Keyterms: []string{"alpha", "sigma"}})

	// Wide screen uses horizontal loom.Split with vertical divider; narrow omits it.
	if wide := screenText(e, 80, 14); !strings.Contains(wide, "│") {
		t.Fatalf("expected horizontal Split divider in 80x14:\n%s", wide)
	}
	if narrow := screenText(e, 40, 14); strings.Contains(narrow, "│") {
		t.Fatalf("did not expect horizontal divider in stacked 40x14:\n%s", narrow)
	}

	// Edit form wraps long transcript across multiple lines via loom.TextEntry.
	press(e, "e")
	if !e.transcript.Focused() || e.keyterms.Focused() {
		t.Fatalf("expected transcript focused on edit start")
	}
	editOut := screenText(e, 80, 14)
	if !strings.Contains(editOut, "alpha beta") || !strings.Contains(editOut, "omicron") {
		t.Fatalf("wrapped TextEntry missing lines:\n%s", editOut)
	}
	press(e, "\t")
	if e.transcript.Focused() || !e.keyterms.Focused() {
		t.Fatalf("expected keyterms focused after Tab")
	}
	press(e, "\x1b")

	// Confirm defaults to No on Enter; arrow/l selects Yes and confirms on Enter.
	press(e, "d")
	confOut := screenText(e, 80, 14)
	if !strings.Contains(confOut, "Yes") || !strings.Contains(confOut, "▶ No") {
		t.Fatalf("expected Confirm Yes / ▶ No prompt:\n%s", confOut)
	}
	press(e, "\r")
	if has, _ := s.Has("wrap"); !has || e.status != "delete cancelled" {
		t.Fatalf("DefaultNo Enter should cancel: has=%v status=%q", has, e.status)
	}
	pressKeys(e, "d", "l", "\r")
	if has, _ := s.Has("wrap"); has || e.status != "deleted wrap" {
		t.Fatalf("Confirm l+Enter should delete: has=%v status=%q", has, e.status)
	}
}
