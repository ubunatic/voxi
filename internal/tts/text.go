package tts

import (
	"strings"
	"unicode"
)

// SplitText turns paragraphs or sentence runs into skippable playback chunks.
func SplitText(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	paragraphs := strings.Split(text, "\n\n")
	chunks := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.Join(strings.Fields(paragraph), " ")
		if paragraph == "" {
			continue
		}
		chunks = append(chunks, splitSentences(paragraph)...)
	}
	if len(chunks) == 0 {
		return nil
	}
	return chunks
}

func splitSentences(text string) []string {
	runes := []rune(text)
	chunks := make([]string, 0, 1)
	start := 0
	for i, r := range runes {
		if r != '.' && r != '?' && r != '!' {
			continue
		}
		if r == '.' && i > 0 && i+1 < len(runes) && unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]) {
			continue
		}
		if i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
			continue
		}
		chunk := strings.TrimSpace(string(runes[start : i+1]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		start = i + 1
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		chunks = append(chunks, tail)
	}
	return chunks
}
