package workrun

import "strings"

// QuickAcknowledgement recognizes whole messages, never actionable fragments.
// Missing confidence costs an ordinary route, not a suppressed useful answer.
func QuickAcknowledgement(request ConversationRequest) (ConversationDecision, bool) {
	if len(request.Text) > 160 || request.AcknowledgementBlocked || request.HasFiles || request.PlanFirst || validateConversationRequest(request) != nil || !AcknowledgementContextSafe(request.Tasks, request.Recent) {
		return ConversationDecision{}, false
	}
	text := strings.ToLower(strings.TrimSpace(request.Text))
	if strings.ContainsAny(text, "?\n\r\t") {
		return ConversationDecision{}, false
	}
	text = strings.TrimRight(text, "!.")
	var kind string
	switch text {
	case "thanks", "thank you", "thanks gator", "thank you gator", "appreciate it", "thanks, that was helpful", "thanks gator, that was helpful", "thank you, that was helpful":
		kind = "appreciation"
	case "haha", "hahaha", "hahahaha", "lol", "haha that's hilarious", "haha, that's hilarious":
		kind = "laughter"
	case "nice, that's great", "that's great", "that was helpful", "great answer":
		kind = "praise"
	case "see you", "goodbye", "bye", "bye gator":
		kind = "farewell"
	case "hi", "hello", "hello gator", "hey gator":
		kind = "greeting"
	default:
		return ConversationDecision{}, false
	}
	_, _, fallback := AcknowledgementPresentation(kind)
	return ConversationDecision{Kind: "reply", Text: fallback, Reaction: kind}, true
}

// Review and uncertain-task context always stays on the ordinary router. An
// outstanding question may be asking for a consequential confirmation.
func AcknowledgementContextSafe(tasks []ConversationTask, recent []ConversationMessage) bool {
	for _, task := range tasks {
		if task.NeedsInspection || task.CanCorrectAction || task.CanRevisePlan || task.CanReviewInputs || (task.State != "running" && task.State != "completed") {
			return false
		}
	}
	for i := len(recent) - 1; i >= 0; i-- {
		if recent[i].Role != "assistant" {
			continue
		}
		text := strings.ToLower(recent[i].Text)
		for _, marker := range []string{"?", "approve", "confirm", "authoriz", "send it", "go ahead", "shall i", "should i", "would you like"} {
			if strings.Contains(text, marker) {
				return false
			}
		}
		break
	}
	return true
}

// The vocabulary is presentation metadata, not a new dispatch kind. Existing
// readers retain the ordinary, nonempty reply text for every acknowledgement.
func AcknowledgementPresentation(kind string) (emoji, level, fallback string) {
	switch kind {
	case "appreciation":
		return "❤️", "subtle", "You're welcome."
	case "laughter":
		return "😂", "subtle", "Haha!"
	case "praise":
		return "🙌", "subtle", "Thanks for the kind words."
	case "farewell":
		return "👋", "subtle", "Take care!"
	case "greeting":
		return "👋", "expressive", "Hello!"
	default:
		return "", "", ""
	}
}
