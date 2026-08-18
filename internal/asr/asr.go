package asr

import (
	"regexp"
	"strings"
)

var (
	ansiEscapeRe   = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	rfc3339TimeRe  = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
	quoteExtractRe  = regexp.MustCompile(`Transcription completed in [^:]+:\s*"([^"]*)"`)
	urlPatternRe    = regexp.MustCompile(`(?i)\b(https?://|www\.)[a-z0-9-]+\.[a-z]+`)
	hallucinationRe = regexp.MustCompile(`(?i)^\s*(thank you for watching|thanks for watching|thank you|thanks for listening|please subscribe.*|subscribe to my channel|see you next time|see you in the next video|subtitles by.*|translated by.*|like and subscribe|mcrun|mbc|learn english.*|.*engvid\.com.*)\s*[.!]?\s*$`)
)

// StripANSI removes all ANSI escape sequences from s.
func StripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
}

// IsSafeToType validates that a text string contains genuine speech and no diagnostic log leakage, URLs, or hallucinations.
func IsSafeToType(text string) bool {
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
	if hallucinationRe.MatchString(trimmed) {
		return false
	}
	return true
}

// StripTrailingHallucinations cleans trailing YouTube / silence artifact tokens.
func StripTrailingHallucinations(text string) string {
	clean := text
	patterns := []string{
		`(?i)\s*thank you for watching[.!]*`,
		`(?i)\s*thanks for watching[.!]*`,
		`(?i)\s*please subscribe[.!]*`,
		`(?i)\s*mcrun[.!]*`,
		`(?i)\s*in the video[.!]*`,
		`(?i)\s*in this video[.!]*`,
		`(?i)\s*in today's video[.!]*`,
		`(?i)\s*learn english.*[.!]*`,
		`(?i)\s*free-to-use tip[.!]*`,
		`(?i)\s*www\.[a-z0-9-]+\.[a-z]+[.!]*`,
		`(?i)\s*and like\.\s*thank you[.!]*`,
	}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		clean = re.ReplaceAllString(clean, "")
	}
	return strings.TrimSpace(clean)
}

// CleanWhisperTranscript extracts only valid human speech from Voxtype transcribe output,
// discarding ANSI escape codes, diagnostic logs, timestamps, model metadata, and hallucinations.
func CleanWhisperTranscript(output string) string {
	clean := StripANSI(output)

	// Strategy 1: Look for Voxtype's explicit canonical summary line:
	// 'Transcription completed in 1.25s: "the quick brown fox"'
	if m := quoteExtractRe.FindStringSubmatch(clean); len(m) > 1 {
		candidate := strings.TrimSpace(m[1])
		candidate = StripTrailingHallucinations(candidate)
		if IsSafeToType(candidate) {
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
		trimmed = StripTrailingHallucinations(trimmed)
		if IsSafeToType(trimmed) {
			resultLines = append(resultLines, trimmed)
		}
	}
	return strings.Join(resultLines, " ")
}
