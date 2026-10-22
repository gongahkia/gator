package workrun

import (
	"context"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
)

type historyPolicyModel struct{ requests []agent.TurnRequest }

func (m *historyPolicyModel) Complete(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
	m.requests = append(m.requests, r)
	return agent.Turn{Text: "Please supply the earlier details."}, nil
}

func TestHistoryPrivacyChangesToolAuthorityAndPolicyDigest(t *testing.T) {
	var priorDigest string
	for _, disabled := range []bool{false, true} {
		m := &historyPolicyModel{}
		r := Request{ForegroundApp: true, DisableChatHistory: disabled, SourcePath: t.TempDir(), Objective: "Recall my earlier decision", Mode: action.Inspect, Contract: artifact.InspectionContract()}
		digest, err := PolicyDigest(r)
		if err != nil {
			t.Fatal(err)
		}
		if digest == priorDigest {
			t.Fatal("history authority omitted from policy identity")
		}
		priorDigest = digest
		// Exercise the service adapter too: defaults must preserve the restriction.
		service := Service{Defaults: &r, Executor: Executor{Model: m, StateDir: t.TempDir()}}
		request := r
		request.DisableChatHistory = false
		if _, err := service.Execute(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if len(m.requests) == 0 {
			t.Fatal("no model request")
		}
		for _, req := range m.requests {
			names := map[string]bool{}
			for _, definition := range req.Tools {
				names[definition.Name] = true
			}
			if names["chat_search"] == disabled || names["chat_read"] == disabled || !names["memory_remember"] {
				t.Fatalf("incorrect tool authority: %#v", names)
			}
			if disabled && (!strings.Contains(req.System, "Earlier-chat retrieval is off") || strings.Contains(req.System, "Use chat_search/chat_read")) {
				t.Fatal("prompt advertised forbidden retrieval")
			}
		}
	}
}
