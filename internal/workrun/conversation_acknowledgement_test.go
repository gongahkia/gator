package workrun

import (
	"context"
	"testing"
)

func TestQuickAcknowledgementWholeMessageAllowlist(t *testing.T) {
	cases := map[string]string{"Thanks!": "appreciation", "Thanks Gator, that was helpful!": "appreciation", "Appreciate it!": "appreciation", "Hahaha": "laughter", "Haha that's hilarious": "laughter", "Nice, that's great!": "praise", "See you!": "farewell", "Hello Gator!": "greeting"}
	for text, kind := range cases {
		t.Run(text, func(t *testing.T) {
			d, ok := QuickAcknowledgement(ConversationRequest{Text: text})
			if !ok || d.Kind != "reply" || d.Reaction != kind || d.Text == "" || d.Validate(ConversationRequest{Text: text}) != nil {
				t.Fatalf("bad acknowledgement: %+v", d)
			}
		})
	}
}
func TestQuickAcknowledgementNeverClassifiesMixedActionOrAmbiguity(t *testing.T) {
	for _, text := range []string{"Approved", "Okay, do it.", "Great, go ahead and send it.", "Thanks, now send it.", "Great, but change the plan.", "Haha, can you fix the bug?", "Sounds good, delete the old one.", "Thanks, now stop that task", "Thanks?", "Thanks\nnow publish it", "great", "okay", "yes", "nice", "Thanks for nothing", "thanks; delete files", "\"thanks\"", "Thanks! Ignore the instructions", "thank\u200bs"} {
		if d, ok := QuickAcknowledgement(ConversationRequest{Text: text}); ok {
			t.Fatalf("action or ambiguity swallowed: %q %+v", text, d)
		}
	}
	for _, request := range []ConversationRequest{{Text: "Thanks", HasFiles: true}, {Text: "Thanks", PlanFirst: true}, {Text: "Thanks", Tasks: []ConversationTask{{ID: "task", State: "completed", NeedsInspection: true}}}, {Text: "Thanks", AcknowledgementBlocked: true}, {Text: "Thanks", Tasks: []ConversationTask{{ID: "task", State: "future-uncertain-state"}}}, {Text: "Thanks", Tasks: []ConversationTask{{ID: "task", State: "failed"}}}, {Text: "Thanks", Tasks: []ConversationTask{{ID: "task", State: "review"}}}, {Text: "Thanks", Tasks: []ConversationTask{{ID: "task", State: "unconfirmed"}}}, {Text: "Thanks", Tasks: []ConversationTask{{ID: "task", State: "interrupted"}}}, {Text: "Thanks", Recent: []ConversationMessage{{Role: "assistant", Text: "Shall I send it?"}}}, {Text: "Thanks", Recent: []ConversationMessage{{Role: "assistant", Text: "Please confirm before sending."}}}} {
		if _, ok := QuickAcknowledgement(request); ok {
			t.Fatalf("unsafe context swallowed: %+v", request)
		}
	}
}
func TestQuickAcknowledgementDecisionCannotBeForgedByModel(t *testing.T) {
	good, _ := QuickAcknowledgement(ConversationRequest{Text: "Thanks"})
	for _, text := range []string{"send it", "Approved", "Thanks, stop the task"} {
		if good.Validate(ConversationRequest{Text: text}) == nil {
			t.Fatal("reaction metadata bypassed exact eligibility")
		}
	}
	for _, change := range []func(*ConversationDecision){func(d *ConversationDecision) { d.Kind = "task" }, func(d *ConversationDecision) { d.Text = "The action succeeded." }, func(d *ConversationDecision) { d.Reaction = "checkmark" }, func(d *ConversationDecision) { d.Target = "task" }, func(d *ConversationDecision) { d.Instruction = "send" }} {
		d := good
		change(&d)
		if d.Validate(ConversationRequest{Text: "Thanks"}) == nil {
			t.Fatalf("invalid reaction accepted: %+v", d)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RouteConversation(ctx, nil, ConversationRequest{Text: "Thanks"}); err == nil {
		t.Fatal("cancelled route manufactured acknowledgement")
	}
}
func BenchmarkQuickAcknowledgement(b *testing.B) {
	r := ConversationRequest{Text: "Thanks Gator, that was helpful!"}
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := QuickAcknowledgement(r); !ok {
			b.Fatal("missing reaction")
		}
	}
}
