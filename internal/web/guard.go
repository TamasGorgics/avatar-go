package web

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"avatar-go/internal/twin"
)

type injectionRule struct {
	reason string
	re     *regexp.Regexp
}

var injectionRules = []injectionRule{
	{
		reason: "instruction_override",
		re:     regexp.MustCompile(`\b(ignore|disregard|forget|override|bypass)\b.{0,60}\b(previous|prior|above|earlier|all|any|your|the|these|those)\b.{0,60}\b(instruction|instructions|rule|rules|direction|directions|guidance|prompt|prompts)\b`),
	},
	{
		reason: "instruction_override",
		re:     regexp.MustCompile(`\b(ignore|disregard|forget)\b.{0,40}\b(everything|anything|all|the)\b.{0,40}\b(above|before|prior|previous)\b`),
	},
	{
		reason: "prompt_reveal",
		re:     regexp.MustCompile(`\b(reveal|show|print|repeat|output|display|leak|expose|tell|give)\b.{0,40}\b(system prompt|system message|hidden prompt|hidden instruction|hidden rule|initial prompt|initial instruction|your instruction|your rule|your prompt|the text above|everything above)\b`),
	},
	{
		reason: "instruction_probe",
		re:     regexp.MustCompile(`\b(system|hidden|initial|secret)\b.{0,20}\b(prompt|instruction|rule)s?\b`),
	},
	{
		reason: "persona_override",
		re:     regexp.MustCompile(`\b(you are now (dan|jailbroken|in developer mode)|from now on|developer mode|jailbreak|do anything now|dan mode|roleplay as|role-play as|pretend to be|pretend you are|act as if you|act as though you)\b`),
	},
}

var compactInjectionRules = []struct {
	reason string
	needle string
}{
	{"instruction_override", "ignorepreviousinstructions"},
	{"instruction_override", "ignoreallprevious"},
	{"instruction_override", "disregardprevious"},
	{"instruction_override", "forgetprevious"},
	{"instruction_override", "ignoreyourrules"},
	{"instruction_override", "forgetyourrules"},
	{"instruction_override", "ignoretheabove"},
	{"instruction_override", "forgettheabove"},
	{"prompt_reveal", "revealyoursystemprompt"},
	{"prompt_reveal", "revealyourinstructions"},
	{"prompt_reveal", "printyourinstructions"},
	{"prompt_reveal", "repeatthetextabove"},
	{"prompt_reveal", "systemprompt"},
	{"persona_override", "youarenowdan"},
	{"persona_override", "youarenowjailbroken"},
	{"persona_override", "developermode"},
	{"persona_override", "jailbreak"},
	{"persona_override", "doanythingnow"},
	{"persona_override", "pretendtobe"},
	{"persona_override", "roleplayas"},
	{"persona_override", "actasifyou"},
	{"persona_override", "actasthoughyou"},
}

// screenHistory checks visitor-authored messages for high-confidence prompt
// injection attempts. Assistant messages are model output; screening them
// would reject ordinary follow-ups when the model refuses an injection.
func screenHistory(history []twin.Message) string {
	var joined strings.Builder
	for _, message := range history {
		if message.Role != "user" {
			continue
		}
		if reason := screenText(message.Content); reason != "" {
			return reason
		}
		joined.WriteString(message.Content)
		joined.WriteByte('\n')
	}
	return screenText(joined.String())
}

func screenText(text string) string {
	normalized := normalizeForScreen(text)
	if normalized == "" {
		return ""
	}
	for _, rule := range injectionRules {
		if rule.re.MatchString(normalized) {
			return rule.reason
		}
	}

	compact := compactForScreen(normalized)
	for _, rule := range compactInjectionRules {
		if strings.Contains(compact, rule.needle) {
			return rule.reason
		}
	}
	return ""
}

func normalizeForScreen(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	lastSpace := false
	for _, r := range text {
		// Map fullwidth ASCII to ASCII so fullwidth spelling cannot evade the
		// phrase checks.
		if r >= 0xFF01 && r <= 0xFF5E {
			r -= 0xFEE0
		}
		switch {
		case unicode.IsSpace(r):
			if !lastSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			lastSpace = true
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			// Drop invisible format and control characters.
		default:
			b.WriteRune(unicode.ToLower(r))
			lastSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

func compactForScreen(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cleanContent removes characters that have no place in a visitor question and
// that can be used to hide instructions from a casual reader.
func cleanContent(content string) string {
	var b strings.Builder
	b.Grow(len(content))
	for _, r := range content {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(r)
		case r == '\u2028' || r == '\u2029':
			b.WriteByte('\n')
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			// Drop invisible format and control characters.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func truncateUTF8(content string, maxBytes int) string {
	if len(content) <= maxBytes {
		return content
	}
	for maxBytes > 0 && !utf8.RuneStart(content[maxBytes]) {
		maxBytes--
	}
	return content[:maxBytes]
}
