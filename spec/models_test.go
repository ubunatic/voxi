package spec

import "testing"

func TestLoadModels(t *testing.T) {
	s, err := LoadModels()
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if _, ok := s.Models[s.DefaultModel]; !ok {
		t.Fatalf("default_model %q not defined in models", s.DefaultModel)
	}
	if len(s.SpeechContext.Terms) == 0 || s.SpeechContext.MaxTerms != 50 {
		t.Fatalf("speech_context not loaded: %+v", s.SpeechContext)
	}
	for name, m := range s.Models {
		if m.Label == "" {
			t.Errorf("model %q: label must not be empty", name)
		}
		if m.Engine == "" {
			t.Errorf("model %q: engine must not be empty", name)
		}
		if len(m.StopWords) == 0 {
			t.Errorf("model %q: stop_words must not be empty", name)
		}
	}
}

func TestAllowsCPU(t *testing.T) {
	s, err := LoadModels()
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if s.AllowsCPU("large-v3-turbo") {
		t.Error(`AllowsCPU("large-v3-turbo") = true, want false (requires_gpu)`)
	}
	if !s.AllowsCPU("base.en") {
		t.Error(`AllowsCPU("base.en") = false, want true`)
	}
	if !s.AllowsCPU("not-a-real-model") {
		t.Error(`AllowsCPU("not-a-real-model") = false, want true (unknown models carry no restriction)`)
	}
}

func TestResolveModel(t *testing.T) {
	s, err := LoadModels()
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}

	resolved, fellBack, err := s.ResolveModel("large-v3-turbo", true)
	if err != nil || fellBack || resolved != "large-v3-turbo" {
		t.Errorf(`ResolveModel("large-v3-turbo", true) = (%q, %v, %v), want ("large-v3-turbo", false, nil)`, resolved, fellBack, err)
	}

	wantFallback := s.Models["large-v3-turbo"].CPUFallback
	if wantFallback == "" {
		t.Fatal("large-v3-turbo has no cpu_fallback configured; update this test's expectations")
	}
	resolved, fellBack, err = s.ResolveModel("large-v3-turbo", false)
	if err != nil || !fellBack || resolved != wantFallback {
		t.Errorf(`ResolveModel("large-v3-turbo", false) = (%q, %v, %v), want (%q, true, nil)`, resolved, fellBack, err, wantFallback)
	}

	resolved, fellBack, err = s.ResolveModel("base.en", false)
	if err != nil || fellBack || resolved != "base.en" {
		t.Errorf(`ResolveModel("base.en", false) = (%q, %v, %v), want ("base.en", false, nil)`, resolved, fellBack, err)
	}
}

func TestLoadModelsRejectsFallbackChain(t *testing.T) {
	yamlDoc := []byte(`
default_model: a
speech_context:
  prompt_prefix: "Terms:"
  terms: [Go]
  max_terms: 10
  max_chars: 100
  max_term_chars: 20
models:
  a:
    label: A
    engine: whisper
    stop_words: [{id: x, pattern: x}]
    requires_gpu: true
    cpu_fallback: b
  b:
    label: B
    engine: whisper
    stop_words: [{id: x, pattern: x}]
    requires_gpu: true
`)
	if _, err := parseModelSpec(yamlDoc); err == nil {
		t.Fatal("expected an error for a cpu_fallback that itself requires_gpu, got nil")
	}
}

func TestStopWordsFallsBackToDefault(t *testing.T) {
	s, err := LoadModels()
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	got := s.StopWords("not-a-real-model")
	want := s.Models[s.DefaultModel].StopWords
	if len(got) != len(want) {
		t.Fatalf("StopWords(unknown) = %d entries, want fallback of %d", len(got), len(want))
	}
}
