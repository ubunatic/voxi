package eager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ubunatic.com/voxi/spec"
)

// writeTestWAV writes a minimal placeholder file to stand in for a wav
// path -- transcribeOpenAIWAV only streams its bytes into a multipart
// body, it never parses audio, so content doesn't matter for these tests.
func writeTestWAV(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.wav")
	if err != nil {
		t.Fatalf("create temp wav: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString("fake-wav-bytes"); err != nil {
		t.Fatalf("write temp wav: %v", err)
	}
	return f.Name()
}

// TestTranscribeOpenAIWAV_PlainText locks in the issue 126 whisper-server
// contract: responseFormat left empty must still send response_format=text
// on the wire and return the response body verbatim (no JSON parsing, no
// marker stripping), so this engine keeps working against the existing
// agy whisper-server user unaffected by the R2T2/json path.
func TestTranscribeOpenAIWAV_PlainText(t *testing.T) {
	var gotFormat string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		gotFormat = r.FormValue("response_format")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	got, err := transcribeOpenAIWAV(context.Background(), writeTestWAV(t), ts.URL, "whisper-1", "", "")
	if err != nil {
		t.Fatalf("transcribeOpenAIWAV: %v", err)
	}
	if gotFormat != "text" {
		t.Errorf("response_format on the wire = %q, want %q", gotFormat, "text")
	}
	if got != "hello world" {
		t.Errorf("transcript = %q, want %q", got, "hello world")
	}
}

// TestTranscribeOpenAIWAV_JSONEnvelope covers the R2T2/llama-server shape
// (issue 134 §6 M1): responseFormat "json" must send response_format=json
// on the wire and parse the {"type":...,"text":...} envelope.
func TestTranscribeOpenAIWAV_JSONEnvelope(t *testing.T) {
	var gotFormat string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		gotFormat = r.FormValue("response_format")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"transcript.text.done","text":"ask not what your country can do for you","usage":{}}`))
	}))
	defer ts.Close()

	got, err := transcribeOpenAIWAV(context.Background(), writeTestWAV(t), ts.URL, "r2t2", "json", "")
	if err != nil {
		t.Fatalf("transcribeOpenAIWAV: %v", err)
	}
	if gotFormat != "json" {
		t.Errorf("response_format on the wire = %q, want %q", gotFormat, "json")
	}
	want := "ask not what your country can do for you"
	if got != want {
		t.Errorf("transcript = %q, want %q", got, want)
	}
}

// TestParseOpenAITranscribeResponse_StripsMultilingualMarker is the
// negative test issue 134 §6 M2 calls for: a literal TrimPrefix on
// "language English<asr_text>" would silently stop working for any other
// language, so this proves the cut-at-marker approach strips the prefix
// regardless of which language name R2T2 emits -- German here, not
// English -- and that the leaked prefix text is entirely gone afterward.
func TestParseOpenAITranscribeResponse_StripsMultilingualMarker(t *testing.T) {
	body := []byte(`{"type":"transcript.text.done","text":"language German<asr_text>Guten Tag"}`)
	got, err := parseOpenAITranscribeResponse(body, "json", "<asr_text>")
	if err != nil {
		t.Fatalf("parseOpenAITranscribeResponse: %v", err)
	}
	if got != "Guten Tag" {
		t.Errorf("transcript = %q, want %q", got, "Guten Tag")
	}
	if strings.Contains(got, "language") || strings.Contains(got, "<asr_text>") {
		t.Errorf("transcript still contains leaked marker/prefix: %q", got)
	}
}

// TestParseOpenAITranscribeResponse_MarkerAbsentIsNoOp ensures a missing
// marker leaves the text untouched rather than truncating or erroring --
// stripBeforeMarker must be a no-op when the marker isn't present.
func TestParseOpenAITranscribeResponse_MarkerAbsentIsNoOp(t *testing.T) {
	got, err := parseOpenAITranscribeResponse([]byte(`{"text":"plain transcript, no marker"}`), "json", "<asr_text>")
	if err != nil {
		t.Fatalf("parseOpenAITranscribeResponse: %v", err)
	}
	if got != "plain transcript, no marker" {
		t.Errorf("transcript = %q, want unchanged text", got)
	}
}

// TestParseOpenAITranscribeResponse_MalformedJSON proves a non-JSON body
// under responseFormat "json" is a hard error, not silently passed through
// as if it were the transcript -- e.g. an unexpected plain-text/error body
// from a misbehaving backend must not be typed to the user.
func TestParseOpenAITranscribeResponse_MalformedJSON(t *testing.T) {
	_, err := parseOpenAITranscribeResponse([]byte("not json"), "json", "")
	if err == nil {
		t.Fatal("expected error for malformed json body, got nil")
	}
}

// TestResolveOpenAIASRBaseURL_ModelWins proves a per-model spec base_url
// (issue 134 review gap: r2t2-confucius4 needs :18131, the global setting
// stays pinned at agy whisper-server's :8090) wins over the global
// OpenAIASRBaseURL setting when both are set.
func TestResolveOpenAIASRBaseURL_ModelWins(t *testing.T) {
	got := resolveOpenAIASRBaseURL("http://127.0.0.1:18131/v1", "http://127.0.0.1:8090/v1")
	if got != "http://127.0.0.1:18131/v1" {
		t.Errorf("resolveOpenAIASRBaseURL = %q, want the model's base_url", got)
	}
}

// TestResolveOpenAIASRBaseURL_FallsBackToGlobal proves a model without its
// own base_url (e.g. openai-transcribe-gemini) still resolves to the
// global OpenAIASRBaseURL setting, so its behaviour is unchanged by this
// feature.
func TestResolveOpenAIASRBaseURL_FallsBackToGlobal(t *testing.T) {
	got := resolveOpenAIASRBaseURL("", "http://127.0.0.1:8090/v1")
	if got != "http://127.0.0.1:8090/v1" {
		t.Errorf("resolveOpenAIASRBaseURL = %q, want the global setting", got)
	}
}

// TestResolveOpenAIASRBaseURL_FallsBackToDefault proves that with neither a
// model base_url nor a global setting, resolveOpenAIASRBaseURL returns ""
// so transcribeOpenAIWAV's own defaultOpenAIASRBaseURL fallback applies.
func TestResolveOpenAIASRBaseURL_FallsBackToDefault(t *testing.T) {
	got := resolveOpenAIASRBaseURL("", "")
	if got != "" {
		t.Errorf("resolveOpenAIASRBaseURL = %q, want empty (defer to transcribeOpenAIWAV default)", got)
	}
}

// TestResolveOpenAIASRBaseURL_SpecEntries locks the resolution order to the
// real spec/models.yaml entries: r2t2-confucius4 must resolve to its own
// pinned base_url regardless of the global setting, and
// openai-transcribe-gemini (no base_url of its own) must still resolve to
// the global setting -- proving M2's single-endpoint gemini behaviour is
// unaffected by this change.
func TestResolveOpenAIASRBaseURL_SpecEntries(t *testing.T) {
	modelSpec, err := spec.LoadModels()
	if err != nil {
		t.Fatalf("spec.LoadModels: %v", err)
	}
	const globalBaseURL = "http://127.0.0.1:8090/v1"

	r2t2, ok := modelSpec.Models["r2t2-confucius4"]
	if !ok {
		t.Fatal("models.yaml missing r2t2-confucius4")
	}
	if got := resolveOpenAIASRBaseURL(r2t2.BaseURL, globalBaseURL); got != "http://127.0.0.1:18131/v1" {
		t.Errorf("r2t2-confucius4 resolved base URL = %q, want its own pinned base_url", got)
	}

	gemini, ok := modelSpec.Models["openai-transcribe-gemini"]
	if !ok {
		t.Fatal("models.yaml missing openai-transcribe-gemini")
	}
	if gemini.BaseURL != "" {
		t.Fatalf("openai-transcribe-gemini has base_url %q, want unset (unchanged behaviour)", gemini.BaseURL)
	}
	if got := resolveOpenAIASRBaseURL(gemini.BaseURL, globalBaseURL); got != globalBaseURL {
		t.Errorf("openai-transcribe-gemini resolved base URL = %q, want the global setting", got)
	}
}
