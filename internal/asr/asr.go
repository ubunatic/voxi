package asr

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	ansiEscapeRe   = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	rfc3339TimeRe  = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
	quoteExtractRe = regexp.MustCompile(`Transcription completed in [^:]+:\s*"([^"]*)"`)
	urlPatternRe   = regexp.MustCompile(`(?i)\b(https?://|www\.)[a-z0-9-]+\.[a-z]+`)

	// leadingDashFragmentRe matches a spurious short dash-prefixed fragment that
	// Whisper sometimes prepends to a genuine sentence, e.g. "-Transcribe. I will…"
	// or "-H. Also file…".
	//
	// The pattern is intentionally tight:
	//   ^-\s*        – literal leading dash, optional space after it
	//   \S+          – one non-whitespace token (the garbled fragment word)
	//   [.,!?]*\s+   – optional punctuation, then mandatory whitespace
	//   (?:[A-Z])    – the remainder MUST start with a capital letter (not consumed)
	//
	// This prevents stripping:
	//   • a transcript that is entirely or mostly a dash-prefixed item (no capital
	//     remainder follows, so the assertion fails)
	//   • legitimate hyphen-led dictated text where the continuation is not a
	//     capital-sentence start (e.g. "- first item in list")
	leadingDashFragmentRe = regexp.MustCompile(`^-\s*\S+[.,!?]*\s+(?:[A-Z])`)
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
	// A punctuation-only decode is a common low-confidence silence/noise
	// artifact (for example, a bare period). It carries no dictated words and
	// must never reach the desktop injector.
	if strings.TrimFunc(trimmed, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	}) == "" {
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

// CollapseRepeatedTrailingClause removes a two-word suffix repeated
// immediately after terminal punctuation, e.g. "Let's get started. get
// started" becomes "Let's get started.". Repetition without a punctuation
// boundary is left untouched so intentional emphasis such as "very very good"
// is preserved. A one-word suffix is never collapsed: it is indistinguishable
// from a legitimate short answer that happens to restate the question's last
// word, e.g. "Was your answer no? No" (see issue 094).
func CollapseRepeatedTrailingClause(text string) string {
	runes := []rune(text)
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] != '.' && runes[i] != '!' && runes[i] != '?' {
			continue
		}
		if i+1 >= len(runes) || !unicode.IsSpace(runes[i+1]) {
			continue
		}

		suffix := strings.TrimSpace(string(runes[i+1:]))
		suffix = strings.TrimSuffix(suffix, ".")
		suffix = strings.TrimSuffix(suffix, "!")
		suffix = strings.TrimSuffix(suffix, "?")
		suffixWords := strings.Fields(suffix)
		if len(suffixWords) != 2 || !allWords(suffixWords) {
			continue
		}

		prefixWords := strings.Fields(string(runes[:i]))
		if len(prefixWords) < len(suffixWords) {
			continue
		}
		matches := true
		start := len(prefixWords) - len(suffixWords)
		for j, suffixWord := range suffixWords {
			if !strings.EqualFold(prefixWords[start+j], suffixWord) {
				matches = false
				break
			}
		}
		if matches {
			return strings.TrimSpace(string(runes[:i+1]))
		}
	}
	return text
}

func allWords(words []string) bool {
	for _, word := range words {
		for _, r := range word {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' && r != '-' {
				return false
			}
		}
	}
	return true
}

// StripLeadingDashFragment removes a spurious short dash-prefixed fragment that
// Whisper sometimes prepends to an otherwise genuine transcript, e.g.:
//
//	"-Transcribe. I will now check…" → "I will now check…"
//	"-H. Also file a follow-up…"    → "Also file a follow-up…"
//
// The heuristic is tight by design: it only strips when ALL of these hold:
//  1. The text begins with a literal '-' (with optional space after it).
//  2. The fragment is a single short token (≤1 word), optionally followed by
//     punctuation and whitespace.
//  3. The remainder immediately starts with an upper-case letter (i.e. it looks
//     like the beginning of a new sentence).
//
// Legitimate hyphen-led text is NOT stripped:
//   - "- first item in list" → unchanged (no capital-sentence continuation)
//   - A transcript that is entirely the fragment (no remainder) → unchanged;
//     callers should let IsSafeToType decide whether to type it.
func StripLeadingDashFragment(text string) string {
	loc := leadingDashFragmentRe.FindStringIndex(text)
	if loc == nil {
		return text
	}
	// loc[1] points just past the capital letter that started the remainder;
	// step back one rune so the capital letter is kept.
	remainder := text[loc[1]-1:]
	return strings.TrimSpace(remainder)
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
		candidate = StripLeadingDashFragment(candidate)
		candidate = StripLeadingHallucinations(candidate, stopWords)
		candidate = StripTrailingHallucinations(candidate, stopWords)
		candidate = CollapseRepeatedTrailingClause(candidate)
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
		trimmed = StripLeadingDashFragment(trimmed)
		trimmed = StripLeadingHallucinations(trimmed, stopWords)
		trimmed = StripTrailingHallucinations(trimmed, stopWords)
		trimmed = CollapseRepeatedTrailingClause(trimmed)
		if IsSafeToType(trimmed, stopWords) {
			resultLines = append(resultLines, trimmed)
		}
	}
	return strings.Join(resultLines, " ")
}
