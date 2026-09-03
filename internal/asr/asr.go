package asr

import (
	"regexp"
	"strings"
)

var (
	ansiEscapeRe   = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	rfc3339TimeRe  = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
	quoteExtractRe = regexp.MustCompile(`Transcription completed in [^:]+:\s*"([^"]*)"`)
	urlPatternRe   = regexp.MustCompile(`(?i)\b(https?://|www\.)[a-z0-9-]+\.[a-z]+`)
)

// hallucinationRegexp builds a whole-line matcher from a model's stop-word
// patterns (see spec/models.yaml). Callers pass the patterns explicitly —
// this package holds no hardcoded stop-word list of its own.
func hallucinationRegexp(stopWords []string) *regexp.Regexp {
	if len(stopWords) == 0 {
		return nil
	}
	return regexp.MustCompile(`(?i)^\s*(` + strings.Join(stopWords, "|") + `)\s*[.!]?\s*$`)
}

// StripANSI removes all ANSI escape sequences from s.
func StripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
}

// IsSafeToType validates that a text string contains genuine speech and no
// diagnostic log leakage, URLs, or hallucinations. stopWords are the active
// model's hallucination patterns (see spec/models.yaml).
func IsSafeToType(text string, stopWords []string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if rfc3339TimeRe.MatchString(trimmed) {
		return false
	}
	if strings.Contains(trimmed, "INFO ") || strings.Contains(trimmed, "DEBUG ") ||
		strings.Contains(trimmed, "WARN ") || strings.Contains(trimmed, "ERROR ") ||
		strings.Contains(trimmed, "whisper_") || strings.Contains(trimmed, "ggml-") {
		return false
	}
	if strings.HasPrefix(trimmed, "Loading audio file:") || strings.HasPrefix(trimmed, "Audio format:") ||
		strings.HasPrefix(trimmed, "Processing ") || strings.HasPrefix(trimmed, "Model loaded in") {
		return false
	}
	// Reject URLs and web domains (common Whisper video hallucinations)
	if urlPatternRe.MatchString(trimmed) {
		return false
	}
	// Reject known Whisper silence hallucinations
	if re := hallucinationRegexp(stopWords); re != nil && re.MatchString(trimmed) {
		return false
	}
	return true
}

// StripTrailingHallucinations cleans trailing hallucinated tokens (e.g.
// YouTube outros, subtitle credits) from otherwise-genuine speech, using
// the active model's stop-word patterns (see spec/models.yaml).
func StripTrailingHallucinations(text string, stopWords []string) string {
	clean := text
	for _, w := range stopWords {
		re := regexp.MustCompile(`(?i)\s*` + w + `\s*[.!]*`)
		clean = re.ReplaceAllString(clean, "")
	}
	return strings.TrimSpace(clean)
}

// StripLeadingHallucinations cleans a leading hallucinated prefix (e.g. a
// garbled "subtitles by ..." credit artifact) from otherwise-genuine speech,
// using the active model's stop-word patterns (see spec/models.yaml). Unlike
// StripTrailingHallucinations, the match is anchored to the start of the
// text so a stop-word-like substring occurring mid-sentence is left alone.
func StripLeadingHallucinations(text string, stopWords []string) string {
	clean := text
	for _, w := range stopWords {
		re := regexp.MustCompile(`(?i)^\s*` + w + `\s*[.!,]*\s*`)
		clean = re.ReplaceAllString(clean, "")
	}
	return strings.TrimSpace(clean)
}

// CleanWhisperTranscript extracts only valid human speech from Voxtype transcribe output,
// discarding ANSI escape codes, diagnostic logs, timestamps, model metadata, and hallucinations.
// stopWords are the active model's hallucination patterns (see spec/models.yaml).
func CleanWhisperTranscript(output string, stopWords []string) string {
	clean := StripANSI(output)

	// Strategy 1: Look for Voxtype's explicit canonical summary line:
	// 'Transcription completed in 1.25s: "the quick brown fox"'
	if m := quoteExtractRe.FindStringSubmatch(clean); len(m) > 1 {
		candidate := strings.TrimSpace(m[1])
		// Strip leading before trailing: both are anchored to their own end
		// of the string, so order doesn't change which hallucinations get
		// caught, but stripping the leading prefix first keeps the
		// remaining text's start clean for readability if a caller
		// inspects the intermediate candidate.
		candidate = StripLeadingHallucinations(candidate, stopWords)
		candidate = StripTrailingHallucinations(candidate, stopWords)
		if IsSafeToType(candidate, stopWords) {
			return candidate
		}
	}

	// Strategy 2: Line-by-line filtering of trailing text
	lines := strings.Split(clean, "\n")
	var resultLines []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		// Discard known log lines and metadata
		if strings.HasPrefix(trimmed, "Loading audio file:") ||
			strings.HasPrefix(trimmed, "Audio format:") ||
			strings.HasPrefix(trimmed, "Processing ") ||
			strings.HasPrefix(trimmed, "whisper_") ||
			strings.Contains(trimmed, "INFO ") ||
			strings.Contains(trimmed, "DEBUG ") ||
			strings.Contains(trimmed, "WARN ") ||
			strings.Contains(trimmed, "ERROR ") ||
			strings.Contains(trimmed, "TRACE ") ||
			strings.Contains(trimmed, "Model loaded in ") ||
			strings.Contains(trimmed, "Using local whisper") ||
			strings.Contains(trimmed, "Loading whisper model") ||
			strings.Contains(trimmed, "Transcription completed in ") ||
			rfc3339TimeRe.MatchString(trimmed) {
			continue
		}
		trimmed = StripLeadingHallucinations(trimmed, stopWords)
		trimmed = StripTrailingHallucinations(trimmed, stopWords)
		if IsSafeToType(trimmed, stopWords) {
			resultLines = append(resultLines, trimmed)
		}
	}
	return strings.Join(resultLines, " ")
}
