package entities

import (
	"unicode"
)

type EntityResolution struct {
	RawToken    string  `json:"raw_token"`
	IsPerson    bool    `json:"is_person"`
	MatchedName string  `json:"matched_name"`
	Confidence  float64 `json:"confidence"`
}

type NeedleResult struct {
	Entities []EntityResolution `json:"detected_entities"`
}

func ReplaceEntities(rawText string, result *NeedleResult, threshold float64) string {
	if result == nil || len(result.Entities) == 0 || rawText == "" {
		return rawText
	}

	currentText := rawText
	for _, entity := range result.Entities {
		if !entity.IsPerson || entity.Confidence < threshold {
			continue
		}
		if entity.RawToken == "" || entity.MatchedName == "" {
			continue
		}

		currentText = replaceTokenAtWordBoundaries(currentText, entity.RawToken, entity.MatchedName)
	}

	return currentText
}

func replaceTokenAtWordBoundaries(src, token, replacement string) string {
	if src == "" || token == "" {
		return src
	}

	srcRunes := []rune(src)
	tokenRunes := []rune(token)
	srcLen := len(srcRunes)
	tokenLen := len(tokenRunes)

	if tokenLen == 0 || srcLen < tokenLen {
		return src
	}

	var buf []rune
	i := 0
	for i <= srcLen-tokenLen {
		match := true
		for j := 0; j < tokenLen; j++ {
			if unicode.ToLower(srcRunes[i+j]) != unicode.ToLower(tokenRunes[j]) {
				match = false
				break
			}
		}

		if match {
			leftOK := i == 0 || !isWordRune(srcRunes[i-1])
			rightOK := (i+tokenLen == srcLen) || !isWordRune(srcRunes[i+tokenLen])

			if leftOK && rightOK {
				buf = append(buf, []rune(replacement)...)
				i += tokenLen
				continue
			}
		}

		buf = append(buf, srcRunes[i])
		i++
	}

	for i < srcLen {
		buf = append(buf, srcRunes[i])
		i++
	}

	return string(buf)
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}
