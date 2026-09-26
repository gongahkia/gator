package modelcatalog

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/model"
)

func TestCodexLoginIsActionableOnlyWhenItsOAuthClientIsConfigured(t *testing.T) {
	t.Setenv("GATOR_CODEX_OAUTH_CLIENT_ID", "")
	started := false
	catalog := newModel(Config{
		StateDir: t.TempDir(),
		BeginOAuthLogin: func(provider string) (OAuthLogin, error) {
			if provider != string(model.Codex) {
				t.Fatalf("provider = %q", provider)
			}
			started = true
			return oauthLoginTestStub{}, nil
		},
	})
	for index, entry := range catalog.cloudModels() {
		if entry.provider == string(model.Codex) {
			catalog.localModels.cloudIndex = index
			if entry.canLogin || !strings.Contains(entry.status, "GATOR_CODEX_OAUTH_CLIENT_ID") {
				t.Fatalf("unconfigured Codex entry = %#v", entry)
			}
			break
		}
	}

	next, command := catalog.updateLocalModels(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	catalog = next.(Model)
	if command != nil || started || !strings.Contains(catalog.notice.text, "GATOR_CODEX_OAUTH_CLIENT_ID") {
		t.Fatalf("unconfigured Codex login = notice %q started %t", catalog.notice.text, started)
	}

	t.Setenv("GATOR_CODEX_OAUTH_CLIENT_ID", "gator-client")
	entry, found := catalog.selectedCloudModel()
	if !found || !entry.canLogin || entry.status != "sign-in available" {
		t.Fatalf("configured Codex entry = %#v found=%t", entry, found)
	}
	next, command = catalog.updateLocalModels(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	catalog = next.(Model)
	if command == nil || !started || catalog.oauthLogin == nil {
		t.Fatalf("configured Codex login did not start: started=%t state=%#v", started, catalog)
	}
}

type oauthLoginTestStub struct{}

func (oauthLoginTestStub) URL() string                    { return "https://example.test/sign-in" }
func (oauthLoginTestStub) Complete(context.Context) error { return nil }
func (oauthLoginTestStub) Cancel()                        {}
