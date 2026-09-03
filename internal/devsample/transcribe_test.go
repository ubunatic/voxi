package devsample

import "testing"

func TestSuggestKeytermsIntersectsVocabulary(t *testing.T) {
	vocab := []string{"Voxi", "voxtype", "dotool", "PipeWire", "Wayland", "unrelated-term"}
	text := "Voxi uses voxtype with dotool on PipeWire."
	got := suggestKeyterms(text, vocab, 64)
	want := []string{"Voxi", "voxtype", "dotool", "PipeWire"}
	if len(got) != len(want) {
		t.Fatalf("suggestKeyterms() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("suggestKeyterms()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSuggestKeytermsCaseInsensitiveAndDeduplicated(t *testing.T) {
	vocab := []string{"Cobra", "cobra", "systemd"}
	got := suggestKeyterms("we use COBRA and systemd here", vocab, 64)
	want := []string{"Cobra", "systemd"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("suggestKeyterms() = %v, want %v", got, want)
	}
}

func TestSuggestKeytermsNoMatchesIsEmpty(t *testing.T) {
	got := suggestKeyterms("nothing recognizable here", []string{"Voxi", "dotool"}, 64)
	if len(got) != 0 {
		t.Fatalf("suggestKeyterms() = %v, want empty", got)
	}
}

func TestNormalizeKeytermsSplitsOnCommaOrPipeAndDedups(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"Voxi, voxtype | dotool", "Voxi|voxtype|dotool"},
		{"Voxi|Voxi|voxi", "Voxi"},
		{"", ""},
		{"   ", ""},
		{"single", "single"},
	}
	for _, c := range cases {
		if got := normalizeKeyterms(c.raw); got != c.want {
			t.Errorf("normalizeKeyterms(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}
