package tts

import (
	"reflect"
	"testing"
)

func TestSplitText(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{name: "empty", text: " \n\t ", want: nil},
		{name: "paragraphs", text: "First paragraph.\n\nSecond paragraph!", want: []string{"First paragraph.", "Second paragraph!"}},
		{name: "sentences", text: "First sentence. Second sentence? Third sentence!", want: []string{"First sentence.", "Second sentence?", "Third sentence!"}},
		{name: "preserves punctuation and Unicode", text: "Grüße, Voxi. Café? Да!", want: []string{"Grüße, Voxi.", "Café?", "Да!"}},
		{name: "does not split decimal", text: "Version 1.2 is current. Next sentence.", want: []string{"Version 1.2 is current.", "Next sentence."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SplitText(tt.text); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SplitText() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
