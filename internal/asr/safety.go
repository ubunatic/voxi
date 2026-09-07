package asr

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"ubunatic.com/voxi/spec"
)

// SafetyResult is privacy-safe metadata for the pre-side-effect transcript gate.
type SafetyResult struct {
	Reason      string
	Chars       int
	Digest      string
	RepeatUnit  string
	RepeatCount int
}

func CheckTranscriptSafety(text string, limits spec.TranscriptSafetySpec) SafetyResult {
	r := []rune(text)
	result := SafetyResult{Chars: len(r), Digest: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(text)))[:23]}
	if len(r) > limits.MaxChars {
		result.Reason = "output_too_long"
		return result
	}
	for _, token := range strings.FieldsFunc(text, unicode.IsSpace) {
		tr := []rune(token)
		if len(tr) > limits.MaxTokenChars {
			result.Reason = "token_too_long"
			if unit, count := repeatedUnit(tr, limits); count > 0 {
				result.RepeatUnit, result.RepeatCount = unit, count
				result.Reason = "pathological_repetition"
			}
			return result
		}
		if unit, count := repeatedUnit(tr, limits); count > 0 {
			result.Reason, result.RepeatUnit, result.RepeatCount = "pathological_repetition", unit, count
			return result
		}
	}
	return result
}

func repeatedUnit(token []rune, limits spec.TranscriptSafetySpec) (string, int) {
	for width := 1; width <= limits.MaxRepeatUnitChars && width*limits.MinRepeatCount <= len(token); width++ {
		for start := 0; start+width*limits.MinRepeatCount <= len(token); start++ {
			count := 1
			for start+(count+1)*width <= len(token) && string(token[start:start+width]) == string(token[start+count*width:start+(count+1)*width]) {
				count++
			}
			if count >= limits.MinRepeatCount && count*width >= limits.MinRepeatedChars {
				unit := string(token[start : start+width])
				if utf8.RuneCountInString(unit) <= limits.MaxRepeatUnitChars {
					return unit, count
				}
			}
		}
	}
	return "", 0
}
