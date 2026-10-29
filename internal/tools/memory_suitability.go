package tools

import (
	"regexp"
	"strings"
)

// This is deliberately a conservative admission filter, not a semantic truth
// classifier. Unsupported wording requires review instead of being treated as
// evidence that a statement is personal, enduring or safe to retain.
var memoryCredential = regexp.MustCompile(`(?i)(\b(password|passphrase|api[_ -]?key|access[_ -]?token|refresh[_ -]?token|private[_ -]?key|seed phrase|recovery phrase|secret key)\b\s*(?:is\b|[:=])|-----BEGIN [A-Z ]*PRIVATE KEY-----|\b(?:sk-[a-z0-9_-]{16,}|gh[pousr]_[a-z0-9]{20,}|AKIA[A-Z0-9]{16})\b)`)
var memoryTemporary = regexp.MustCompile(`(?i)\b(for (?:this|the next) (?:reply|answer|task|request|conversation|chat)|(?:today|tonight|this week|this time|right now|temporarily|just this once)|(?:if|suppose|imagine|pretend|hypothetically|roleplay|role-play)\b)`)
var memoryReported = regexp.MustCompile(`(?i)(\b(?:said|says|wrote|writes|quoted|quote|example|translate|translation|fictional|character)\b|\b(?:he|she|they|my (?:friend|colleague|partner|wife|husband|child|client|customer))\b|["“”«»]|(?m)^\s*>|` + "```" + `)`)
var memorySensitive = regexp.MustCompile(`(?i)\b(?:diagnos(?:ed|is)|medical|health|disease|allerg(?:y|ic)|medication|therapy|religion|religious|political|sexual|salary|income|debt|bank account|passport|social security|ssn)\b`)
var memoryPersonal = regexp.MustCompile(`(?i)^\s*(?:I (?:prefer|like|dislike|live|work|speak|use|am|have|need|avoid)\b|my (?:name|timezone|time zone|language|pronouns|job|role|home|city|occupation)\b)`)

func memorySuitability(message, quote string) (deny, review string) {
	// Examine the whole message: extracting a harmless-looking substring cannot
	// strip away its ownership, duration, negative intent or secret context.
	if memoryCredential.MatchString(message) {
		return "credentials belong in a credential store, not personal memory", ""
	}
	for _, negative := range []string{"不要记住", "别记住", "不要保存", "no recuerdes", "no guardes", "jangan ingat", "jangan simpan", "ne mémorise pas", "ne retiens pas"} {
		if strings.Contains(strings.ToLower(message), negative) {
			return "this message asks not to retain memory", ""
		}
	}
	if memoryTemporary.MatchString(message) {
		return "", "The message may describe a temporary request or hypothetical situation. Confirm whether these exact words should apply to future chats."
	}
	if memoryReported.MatchString(message) {
		return "", "The message contains quotation or another person's context. Confirm whose information this is and whether it belongs in your memory."
	}
	if memorySensitive.MatchString(message) {
		return "", "This may be sensitive personal information. Saving it for future chats requires your confirmation."
	}
	if !memoryPersonal.MatchString(strings.TrimSpace(quote)) {
		return "", "The host cannot establish that these words describe an enduring fact or preference about you. Review before saving."
	}
	return "", ""
}
