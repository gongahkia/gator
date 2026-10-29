package memory

// Recognized concept domains support passive capture. This is a schema, not a
// semantic classifier: it cannot prove that a model assigned the right domain.
// Unknown labels require a person to review their possible overlap before they
// can become additional authoritative memories. Trusted manual store APIs stay
// separate from the agent's admission boundary.
func ConceptReviewReason(subject, kind string) string {
	expected := ""
	switch CanonicalSubject(subject) {
	case "profile.name", "profile.pronouns", "profile.city", "profile.timezone", "profile.language", "profile.job", "profile.role", "profile.employer", "profile.workplace":
		expected = "fact"
	case "preference.response_length", "preference.language", "preference.format", "preference.tone", "preference.units", "preference.accessibility":
		expected = "preference"
	}
	if expected == "" {
		return "This memory category is not recognized. It may duplicate, contradict or relabel existing or previously forgotten information. Review the exact words and existing categories before saving it as a separate memory."
	}
	if kind != expected {
		return "The proposed memory kind does not match this category. Review its meaning before saving."
	}
	return ""
}
