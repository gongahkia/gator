package workrun

import (
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/agent"
	"testing"
)

func TestRequestRecoveryRetainsStructuredStatusAndEffectSafety(t *testing.T) {
	for code, kind := range map[int]string{401: "credentials", 403: "model_access", 404: "model_access", 402: "provider_budget", 429: "rate_limit"} {
		for _, safe := range []bool{true, false} {
			diagnostic := errors.New("private provider diagnostic")
			err := fmt.Errorf("execute: %w", agent.HTTPStatusError(code, diagnostic))
			recovery := classifyRequestRecovery(err, safe)
			if recovery == nil || recovery.Kind != kind || recovery.Status != code || recovery.RetrySafe != safe {
				t.Fatal(code, safe, recovery)
			}
			if recovery.Summary == diagnostic.Error() || recovery.Repair == diagnostic.Error() {
				t.Fatal("provider diagnostic leaked into recovery")
			}
		}
	}
	for _, err := range []error{nil, errors.New("HTTP 401"), agent.HTTPStatusError(500, errors.New("server failed")), errors.New("budget exceeded")} {
		if recovery := classifyRequestRecovery(err, true); recovery != nil {
			t.Fatal("parsed prose or invented safe recovery", recovery)
		}
	}
	if recovery := classifyRequestRecovery(agent.ErrBudget, true); recovery == nil || recovery.RetrySafe || recovery.Kind != "work_budget" {
		t.Fatal("exhausted budget reset", recovery)
	}
}
