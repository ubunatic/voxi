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

// splitCapped re-splits one sentence chunk into pieces of at most maxRunes
// runes, breaking at clause boundaries (,;:) and, for clauses still over the
// cap, at word boundaries. Used by the tts-serve backend only (issue 155 M3
// pre-work); SplitText's sentence chunking for Piper is unaffected. maxRunes
// <= 0 disables the cap.
func splitCapped(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 || len([]rune(text)) <= maxRunes {
		return []string{text}
	}
	chunks := make([]string, 0, 4)
	var buf strings.Builder
	bufLen := 0
	flush := func() {
		if bufLen > 0 {
			chunks = append(chunks, strings.TrimSpace(buf.String()))
			buf.Reset()
			bufLen = 0
		}
	}
	appendPiece := func(piece string) {
		pieceLen := len([]rune(piece))
		if bufLen > 0 && bufLen+1+pieceLen > maxRunes {
			flush()
		}
		if bufLen > 0 {
			buf.WriteString(" ")
			bufLen++
		}
		buf.WriteString(piece)
		bufLen += pieceLen
	}
	for _, clause := range splitClauses(text) {
		if len([]rune(clause)) > maxRunes {
			flush()
			for _, word := range strings.Fields(clause) {
				appendPiece(word)
			}
			continue
		}
		appendPiece(clause)
	}
	flush()
	return chunks
}

// splitClauses breaks text at commas, semicolons, and colons followed by
// whitespace, keeping the punctuation with the preceding clause.
func splitClauses(text string) []string {
	runes := []rune(text)
	clauses := make([]string, 0, 4)
	start := 0
	for i, r := range runes {
		if r != ',' && r != ';' && r != ':' {
			continue
		}
		if i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
			continue
		}
		clause := strings.TrimSpace(string(runes[start : i+1]))
		if clause != "" {
			clauses = append(clauses, clause)
		}
		start = i + 1
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		clauses = append(clauses, tail)
	}
	return clauses
}
