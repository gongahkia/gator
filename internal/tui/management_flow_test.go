package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
)

func TestManagementHubLoadsTypedStateAndConfirmsTrustChanges(t *testing.T) {
	backend := &fakeManagementBackend{snapshot: ManagementSnapshot{
		Trusts:     []ManagedTrust{{Kind: "hooks", Hash: "abc123", Configured: true}},
		Worktrees:  []ManagedWorktree{{ID: "run-1", Path: "/tmp/repo-gator-runs/run-1"}},
		Children:   []ManagedChild{{ID: "child-1", Status: "completed", Role: "writer", PatchBytes: 42}},
		Extensions: []ManagedExtension{{ID: "review", Name: "Review", Enabled: true, Tools: 1}},
	}}
	model := New(Config{Management: backend})
	model.width, model.height = 100, 40
	model.task.SetValue("/manage")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.screen != managementScreen || command == nil {
		t.Fatalf("open management = screen:%d command:%v", model.screen, command)
	}
	next, _ = model.Update(command())
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = next.(Model)
	if view := model.View(); !strings.Contains(view, "hooks") || !strings.Contains(view, "abc123") {
		t.Fatalf("management view = %q", view)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	model = next.(Model)
	if model.management.confirm == nil {
		t.Fatal("trust action did not require confirmation")
	}
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = next.(Model)
	if command == nil {
		t.Fatal("confirmed trust action did not start")
	}
	next, command = model.Update(command())
	model = next.(Model)
	if len(backend.trustChanges) != 1 || backend.trustChanges[0] != "hooks:true" {
		t.Fatalf("trust changes = %#v", backend.trustChanges)
	}
	if command == nil {
		t.Fatal("successful action did not refresh management state")
	}
}

func TestManagementSettingsPersistOnlyAfterConfirmation(t *testing.T) {
	backend := &fakeManagementBackend{snapshot: ManagementSnapshot{
		Settings: ManagedSettings{SandboxMode: "strict", Network: "deny", DefaultProvider: "openai", DefaultModel: "gpt"},
	}}
	model := New(Config{Management: backend})
	model.width, model.height = 100, 40
	model.task.SetValue("/manage")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	next, _ = model.Update(command())
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.management.confirm == nil || len(backend.policyChanges) != 0 {
		t.Fatalf("sandbox change before confirmation = confirm:%#v changes:%#v", model.management.confirm, backend.policyChanges)
	}
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = next.(Model)
	next, _ = model.Update(command())
	if got, want := backend.policyChanges, []string{"off:deny"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("policy changes = %#v, want %#v", got, want)
	}
}

func TestManagementApplyRequiresSuccessfulCheckAndSeparateConfirmation(t *testing.T) {
	backend := &fakeManagementBackend{snapshot: ManagementSnapshot{
		Runs: []ManagedRun{{ID: "run-1", StatePath: "/state/run-1", Provider: "openai", Model: "gpt", Available: true}},
	}}
	model := New(Config{Management: backend})
	model.width, model.height = 100, 40
	model.task.SetValue("/manage")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	next, _ = model.Update(command())
	model = next.(Model)
	for range 3 {
		next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
		model = next.(Model)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = next.(Model)
	if model.management.confirm != nil || len(backend.applied) != 0 {
		t.Fatalf("unchecked apply = confirm:%#v applied:%#v", model.management.confirm, backend.applied)
	}
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	model = next.(Model)
	if command == nil {
		t.Fatal("compatibility check did not start")
	}
	next, refresh := model.Update(command())
	model = next.(Model)
	if model.management.checkedRunRecord != "/state/run-1" || refresh == nil {
		t.Fatalf("check state = %q refresh:%v", model.management.checkedRunRecord, refresh)
	}
	next, _ = model.Update(refresh())
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = next.(Model)
	if model.management.confirm == nil || len(backend.applied) != 0 {
		t.Fatalf("checked apply confirmation = %#v applied:%#v", model.management.confirm, backend.applied)
	}
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = next.(Model)
	next, _ = model.Update(command())
	if got, want := backend.applied, []string{"/state/run-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("applied runs = %#v, want %#v", got, want)
	}
}

func TestManagementRefreshClearsCompatibilityCheck(t *testing.T) {
	backend := &fakeManagementBackend{snapshot: ManagementSnapshot{
		Runs: []ManagedRun{{ID: "run-1", StatePath: "/state/run-1", Provider: "openai", Model: "gpt", Available: true}},
	}}
	model := New(Config{Management: backend})
	model.width, model.height = 100, 40
	model.task.SetValue("/manage")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	next, _ = model.Update(command())
	model = next.(Model)
	for range 3 {
		next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
		model = next.(Model)
	}
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	model = next.(Model)
	next, refresh := model.Update(command())
	model = next.(Model)
	next, _ = model.Update(refresh())
	model = next.(Model)
	if model.management.checkedRunRecord != "/state/run-1" {
		t.Fatalf("checked run = %q", model.management.checkedRunRecord)
	}
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = next.(Model)
	if command == nil || model.management.checkedRunRecord != "" {
		t.Fatalf("refresh did not clear check: command=%v checked=%q", command, model.management.checkedRunRecord)
	}
	next, _ = model.Update(command())
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = next.(Model)
	if model.management.confirm != nil || len(backend.applied) != 0 {
		t.Fatalf("apply after refresh = confirm:%#v applied:%#v", model.management.confirm, backend.applied)
	}
}

func openManagementMCPAuth(t *testing.T, backend *fakeManagementBackend) Model {
	t.Helper()
	model := New(Config{Management: backend})
	model.width, model.height = 100, 40
	model.task.SetValue("/manage")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command == nil {
		t.Fatal("open management did not load a snapshot")
	}
	next, _ = model.Update(command())
	model = next.(Model)
	for range 2 {
		next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
		model = next.(Model)
	}
	if model.management.section != managementMCPAuth {
		t.Fatalf("section = %d, want MCP auth", model.management.section)
	}
	return model
}

func TestManagementMCPLoginRequiresTrustedManifestBeforeAuthorizationURL(t *testing.T) {
	backend := &fakeManagementBackend{snapshot: ManagementSnapshot{
		MCPAuth: []ManagedMCPAuth{{Server: "docs", BundleTrusted: false}},
	}}
	model := openManagementMCPAuth(t, backend)
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	model = next.(Model)
	if command != nil || len(backend.mcpLogins) != 0 {
		t.Fatalf("untrusted login started: command=%v logins=%#v", command, backend.mcpLogins)
	}
	if model.oauthLogin != nil || model.management.mcpLoginURL != "" {
		t.Fatalf("untrusted login exposed a URL: %q", model.management.mcpLoginURL)
	}
	if view := model.View(); !strings.Contains(view, "Trust the current .gator/mcp.json hash") {
		t.Fatalf("untrusted login view = %q", view)
	}
}

func TestManagementMCPLoginShowsURLOnlyAfterTrustAndStoresOnCompletion(t *testing.T) {
	backend := &fakeManagementBackend{snapshot: ManagementSnapshot{
		MCPAuth: []ManagedMCPAuth{{Server: "docs", BundleTrusted: true}},
	}}
	model := openManagementMCPAuth(t, backend)
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	model = next.(Model)
	if command == nil || len(backend.mcpLogins) != 1 || backend.mcpLogins[0] != "docs" {
		t.Fatalf("trusted login = command:%v logins:%#v", command, backend.mcpLogins)
	}
	view := model.View()
	if !strings.Contains(view, "https://authorization.example/authorize?server=docs") {
		t.Fatalf("authorization URL missing from view: %q", view)
	}
	next, refresh := model.Update(command())
	model = next.(Model)
	if !backend.mcpLoginStarted.completed {
		t.Fatal("login did not complete through the backend flow")
	}
	if model.oauthLogin != nil || model.management.mcpLoginURL != "" {
		t.Fatalf("completed login left state: login=%v url=%q", model.oauthLogin, model.management.mcpLoginURL)
	}
	if refresh == nil {
		t.Fatal("completed login did not refresh authorization state")
	}
}

func TestManagementMCPCredentialRemovalRequiresConfirmation(t *testing.T) {
	backend := &fakeManagementBackend{snapshot: ManagementSnapshot{
		MCPAuth: []ManagedMCPAuth{{Server: "docs", Authenticated: true, BundleTrusted: true}},
	}}
	model := openManagementMCPAuth(t, backend)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = next.(Model)
	if model.management.confirm == nil || len(backend.mcpRemovals) != 0 {
		t.Fatalf("removal before confirmation = confirm:%#v removals:%#v", model.management.confirm, backend.mcpRemovals)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = next.(Model)
	if len(backend.mcpRemovals) != 0 {
		t.Fatalf("cancelled removal ran: %#v", backend.mcpRemovals)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = next.(Model)
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = next.(Model)
	if command == nil {
		t.Fatal("confirmed removal did not start")
	}
	next, _ = model.Update(command())
	if got, want := backend.mcpRemovals, []string{"docs"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removals = %#v, want %#v", got, want)
	}
}

func openManagementExtensions(t *testing.T, backend *fakeManagementBackend) Model {
	t.Helper()
	model := New(Config{Management: backend})
	model.width, model.height = 100, 40
	model.task.SetValue("/manage")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	next, _ = model.Update(command())
	model = next.(Model)
	for range 6 {
		next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
		model = next.(Model)
	}
	if model.management.section != managementExtensions {
		t.Fatalf("section = %d, want extensions", model.management.section)
	}
	return model
}

func TestManagementExtensionInstallRejectsSSHAndHTTPBeforePrepare(t *testing.T) {
	backend := &fakeManagementBackend{}
	model := openManagementExtensions(t, backend)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	model = next.(Model)
	if model.management.installSource == nil {
		t.Fatal("install did not open source entry")
	}
	model.management.installSource.SetValue("git@example.com:team/ext.git")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command != nil || len(backend.preparedSources) != 0 {
		t.Fatalf("ssh source reached prepare: command=%v sources=%#v", command, backend.preparedSources)
	}
	model.management.installSource.SetValue("http://example.com/ext.git")
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command != nil || len(backend.preparedSources) != 0 {
		t.Fatalf("http source reached prepare: command=%v sources=%#v", command, backend.preparedSources)
	}
}

func TestManagementExtensionInstallCommitsReviewedHashAndDiscardsOnCancel(t *testing.T) {
	source := t.TempDir()
	backend := &fakeManagementBackend{
		preparePreview: ExtensionInstallPreview{Token: ".prepare-test", ID: "review-helper", Name: "Review helper", Hash: "abc123", Tools: 2},
	}
	model := openManagementExtensions(t, backend)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	model = next.(Model)
	model.management.installSource.SetValue(source)
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command == nil {
		t.Fatal("valid local source did not start prepare")
	}
	next, _ = model.Update(command())
	model = next.(Model)
	if model.management.confirm == nil || !strings.Contains(model.management.confirm.label, "abc123") || !strings.Contains(model.View(), "tools=2") {
		t.Fatalf("preview confirm = %#v view=%q", model.management.confirm, model.View())
	}
	if len(backend.committed) != 0 {
		t.Fatalf("prepare committed: %#v", backend.committed)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = next.(Model)
	if len(backend.committed) != 0 || !reflect.DeepEqual(backend.discarded, []string{".prepare-test"}) {
		t.Fatalf("cancel commit=%#v discard=%#v", backend.committed, backend.discarded)
	}

	backend.discarded = nil
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	model = next.(Model)
	model.management.installSource.SetValue(source)
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	next, _ = model.Update(command())
	model = next.(Model)
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = next.(Model)
	if command == nil {
		t.Fatal("confirmed install did not start")
	}
	next, _ = model.Update(command())
	if got, want := backend.committed, []string{".prepare-test:abc123:false"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("committed = %#v, want %#v", got, want)
	}
}

func TestManagementShowsWriterBatchConflictEvidence(t *testing.T) {
	model := New(Config{})
	model.width, model.height = 100, 40
	model.screen = managementScreen
	model.management.section = managementChildren
	model.management.data.Batches = []ManagedBatch{{
		ID: "batch-1", Status: "completed", ChildIDs: []string{"child-a", "child-b"},
		ComparisonStatus: "conflict", ComparisonDetail: "Git three-way comparison reported conflicts",
		Conflicts: []ManagedConflict{{Kind: "path_overlap", ChildIDs: []string{"child-a", "child-b"}, Paths: []string{"internal/shared.go"}, Detail: "both writers changed the same file"}},
	}}
	view := model.View()
	for _, expected := range []string{"batch-1", "comparison=conflict", "three-way comparison: conflict", "Git three-way comparison", "path_overlap", "internal/shared.go", "both writers changed"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("batch conflict view missing %q: %q", expected, view)
		}
	}
}

func TestBrowserActionTimelineHidesSubmittedFieldValues(t *testing.T) {
	detail := describeToolCall(agent.ToolCall{
		Name:      "browser_act",
		Arguments: json.RawMessage(`{"action":"submit","ref":"form-1","fields":{"q":"do-not-render"}}`),
	})
	if strings.Contains(detail, "do-not-render") || !strings.Contains(detail, "fields: 1") || !strings.Contains(detail, "values hidden") {
		t.Fatalf("browser action timeline detail = %q", detail)
	}
}

type fakeManagementBackend struct {
	snapshot        ManagementSnapshot
	trustChanges    []string
	policyChanges   []string
	applied         []string
	mcpLogins       []string
	mcpRemovals     []string
	mcpLoginErr     error
	mcpLoginStarted *stubOAuthLogin
	preparedSources []string
	preparePreview  ExtensionInstallPreview
	prepareErr      error
	committed       []string
	discarded       []string
}

func (backend *fakeManagementBackend) Snapshot(string) (ManagementSnapshot, error) {
	return backend.snapshot, nil
}

func (backend *fakeManagementBackend) SetTrust(kind string, trusted bool) error {
	backend.trustChanges = append(backend.trustChanges, fmt.Sprintf("%s:%t", kind, trusted))
	return nil
}

func (backend *fakeManagementBackend) SetExecutionPolicy(mode, network string) error {
	backend.policyChanges = append(backend.policyChanges, mode+":"+network)
	return nil
}
func (*fakeManagementBackend) SetDefaults(string, string) error       { return nil }
func (*fakeManagementBackend) SetExtensionEnabled(string, bool) error { return nil }
func (*fakeManagementBackend) RemoveExtension(string) error           { return nil }
func (backend *fakeManagementBackend) PrepareExtension(source string) (ExtensionInstallPreview, error) {
	backend.preparedSources = append(backend.preparedSources, source)
	if backend.prepareErr != nil {
		return ExtensionInstallPreview{}, backend.prepareErr
	}
	preview := backend.preparePreview
	if preview.Token == "" {
		preview = ExtensionInstallPreview{Token: ".prepare-test", Source: source, ID: "review-helper", Name: "Review helper", Hash: "abc123", Tools: 2}
	}
	preview.Source = source
	return preview, nil
}
func (backend *fakeManagementBackend) CommitExtensionInstall(token, hash string, replace bool) error {
	backend.committed = append(backend.committed, fmt.Sprintf("%s:%s:%t", token, hash, replace))
	return nil
}
func (backend *fakeManagementBackend) DiscardExtensionPrepare(token string) error {
	backend.discarded = append(backend.discarded, token)
	return nil
}
func (*fakeManagementBackend) PruneWorktrees() error       { return nil }
func (*fakeManagementBackend) RemoveWorktree(string) error { return nil }
func (*fakeManagementBackend) ExportArtifact(string, string) (string, error) {
	return "/tmp/export", nil
}
func (*fakeManagementBackend) CheckPatch(string) (int, error) { return 42, nil }
func (backend *fakeManagementBackend) ApplyPatch(run string) (int, error) {
	backend.applied = append(backend.applied, run)
	return 42, nil
}

func (backend *fakeManagementBackend) BeginMCPOAuthLogin(server string) (OAuthLogin, error) {
	backend.mcpLogins = append(backend.mcpLogins, server)
	if backend.mcpLoginErr != nil {
		return nil, backend.mcpLoginErr
	}
	login := &stubOAuthLogin{url: "https://authorization.example/authorize?server=" + server}
	backend.mcpLoginStarted = login
	return login, nil
}

func (backend *fakeManagementBackend) RemoveMCPCredential(server string) error {
	backend.mcpRemovals = append(backend.mcpRemovals, server)
	return nil
}

type stubOAuthLogin struct {
	url       string
	completed bool
	cancelled bool
}

func (login *stubOAuthLogin) URL() string { return login.url }

func (login *stubOAuthLogin) Complete(context.Context) error {
	login.completed = true
	return nil
}

func (login *stubOAuthLogin) Cancel() { login.cancelled = true }
