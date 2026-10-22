package workrun

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type appControllerFixture struct {
	review  bool
	applied int
}

func (f *appControllerFixture) Inspect(context.Context, AppQuery) (any, error) {
	return map[string]string{"model": "fixture"}, nil
}
func (f *appControllerFixture) Prepare(context.Context, AppChange) (AppMutation, error) {
	return AppMutation{Review: f.review, Preview: map[string]string{"setting": "theme", "after": "mono"}, Apply: func(context.Context) (any, error) {
		f.applied++
		return "saved", nil
	}}, nil
}

func TestAppToolsAreForegroundOnly(t *testing.T) {
	f := &appControllerFixture{}
	if len(applicationTools(f, Request{})) != 0 || len(applicationTools(nil, Request{ForegroundApp: true})) != 0 {
		t.Fatal("app controls escaped the foreground adapter")
	}
	if len(applicationTools(f, Request{ForegroundApp: true})) != 2 {
		t.Fatal("foreground app did not receive inspection and update tools")
	}
}

func TestAppChangesRespectExactReviewAndReportAppliedResults(t *testing.T) {
	for _, decision := range []string{"unavailable", "declined", "approved"} {
		t.Run(decision, func(t *testing.T) {
			f := &appControllerFixture{review: true}
			request := Request{ForegroundApp: true}
			if decision != "unavailable" {
				request.ApproveAppChange = func(_ context.Context, preview any) (bool, error) {
					if preview.(map[string]string)["after"] != "mono" {
						t.Fatal("review lost the exact change")
					}
					return decision == "approved", nil
				}
			}
			result, err := applicationTools(f, request)[1].Execute(context.Background(), json.RawMessage(`{"setting":"theme","value":{"theme":"mono"}}`))
			if decision == "unavailable" && err == nil {
				t.Fatal("review-required change ran without a review channel")
			}
			if decision == "approved" {
				if err != nil || f.applied != 1 || !strings.Contains(result.Content, `"status":"applied"`) {
					t.Fatalf("approved result = %s, %v; applied=%d", result.Content, err, f.applied)
				}
			} else if f.applied != 0 {
				t.Fatal("unapproved change executed")
			}
		})
	}
}

func TestAppToolsRejectUnknownAndTrailingArguments(t *testing.T) {
	f := &appControllerFixture{}
	tool := applicationTools(f, Request{ForegroundApp: true})[1]
	for _, args := range []string{`null`, `{"setting":"theme","value":{},"command":"rm"}`, `{"setting":"theme","value":{}} {}`} {
		if _, err := tool.Execute(context.Background(), json.RawMessage(args)); err == nil {
			t.Fatalf("accepted invalid arguments %s", args)
		}
	}
	if f.applied != 0 {
		t.Fatal("invalid request changed the app")
	}
}
