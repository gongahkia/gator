package cliagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

func TestEveryCLIReportsProcessFailureWithoutOutput(t *testing.T) {
	for _, kind := range []string{"codex", "claude", "pi"} {
		t.Run(kind, func(t *testing.T) {
			r := fixtureRequest(t, `printf '%s\n' "synthetic-private-key-value PRIVATE_PROMPT" >&2; exit 7`)
			r.Config.Kind = kind
			r.Config.Model = "fixture-model"
			var eventDiagnostic *agent.ProviderDiagnostic
			r.OnEvent = func(e agent.Event) {
				if e.ProviderDiagnostic != nil {
					eventDiagnostic = e.ProviderDiagnostic
				}
			}
			_, err := Run(context.Background(), r)
			d := agent.ProviderDiagnosticFrom(err)
			if d == nil || d.Provider != kind || d.Transport != "cli" || d.Method != "exec" || d.Phase != "model turn" || d.ExitCode == nil || *d.ExitCode != 7 || eventDiagnostic == nil {
				t.Fatalf("missing CLI diagnostic: %v", err)
			}
			data, _ := json.Marshal(d)
			if strings.Contains(err.Error(), "synthetic-private-key-value") || strings.Contains(err.Error(), "PRIVATE_PROMPT") || strings.Contains(string(data), `"status"`) {
				t.Fatalf("unsafe or invented metadata: %v", err)
			}
		})
	}
}

func TestCLIDiagnosticPreservesCancellationAndSanitizesEndpoint(t *testing.T) {
	t.Setenv("FIXTURE_API_KEY", "synthetic-private-key-value")
	cfg := Config{Kind: "pi", APIKeyEnv: "FIXTURE_API_KEY", BaseURL: "https://user:password@example.test/synthetic-private-key-value?secret=PRIVATE#fragment"}
	err := cliFailure(cfg, "rpc", "model turn", time.Now(), context.Canceled, nil)
	d := agent.ProviderDiagnosticFrom(err)
	if !errors.Is(err, context.Canceled) || d.Code != "cli_cancelled" || d.Endpoint != "https://example.test/%5Bredacted%5D" {
		t.Fatalf("wrong cancellation diagnostic: %v", err)
	}
}

func TestMissingCLICredentialDoesNotClaimAuthentication(t *testing.T) {
	t.Setenv("FIXTURE_API_KEY", "")
	err := cliFailure(Config{Kind: "pi", APIKeyEnv: "FIXTURE_API_KEY"}, "exec", "configuration", time.Now(), errors.New("missing credential"), nil)
	d := agent.ProviderDiagnosticFrom(err)
	if d.CredentialSource != "none" || d.Auth != "none" {
		t.Fatalf("invented credential metadata: %+v", d)
	}
}
