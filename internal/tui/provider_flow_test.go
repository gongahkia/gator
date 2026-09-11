package tui

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/auth"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func TestProviderDropdownSelectsProviderAndRecommendedModel(t *testing.T) {
	model := New(Config{Provider: "openai", Model: "gpt-5.6"})
	model.focus = providerField
	model.normalizeDropdownSelection()
	for index, option := range model.dropdownOptions() {
		if option.value == "anthropic" {
			model.dropdownIndex = index
			break
		}
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command == nil {
		t.Fatal("provider dropdown did not focus the model field")
	}
	updated := next.(Model)
	if updated.focus != modelField || updated.provider.Value() != "anthropic" || updated.model.Value() != "claude-sonnet-5" {
		t.Fatalf("composer = provider %q, model %q, focus %v", updated.provider.Value(), updated.model.Value(), updated.focus)
	}
	updated.width = 100
	updated.height = 40
	updated.resizeInputs()
	if !updated.drawerOpen || !strings.Contains(updated.View(), "Control center") || !strings.Contains(updated.View(), "claude-sonnet-5") {
		t.Fatalf("model dropdown missing from view: %s", updated.View())
	}
}

func TestProviderDropdownShowsSubscriptionSignInState(t *testing.T) {
	stateDir := t.TempDir()
	credentials, err := auth.New(stateDir)
	if err != nil {
		t.Fatalf("new credentials: %v", err)
	}
	if err := credentials.Put("codex", auth.Credential{Type: "oauth", Access: "token", Expires: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
		t.Fatalf("store credential: %v", err)
	}
	model := New(Config{StateDir: stateDir})
	model.focus = providerField
	for _, option := range model.dropdownOptions() {
		if option.value == "codex" {
			if !strings.Contains(option.description, "signed in") {
				t.Fatalf("Codex provider description = %q", option.description)
			}
			return
		}
	}
	t.Fatal("Codex provider option was missing")
}

func TestProviderDropdownOffersVendorCLIWhenNativeOAuthIsUnconfigured(t *testing.T) {
	t.Setenv("GATOR_CODEX_OAUTH_CLIENT_ID", "")
	model := New(Config{StateDir: t.TempDir()})
	model.focus = providerField
	for _, option := range model.dropdownOptions() {
		if option.value == "codex" {
			if !strings.Contains(option.description, "gator connect codex") {
				t.Fatalf("Codex provider description = %q", option.description)
			}
			return
		}
	}
	t.Fatal("Codex provider option was missing")
}

func TestModelCatalogExplainsVendorCLIPathWhenNativeOAuthIsUnconfigured(t *testing.T) {
	t.Setenv("GATOR_COPILOT_OAUTH_CLIENT_ID", "")
	model := New(Config{
		Provider:    "copilot",
		LocalModels: &fakeLocalModelManager{},
		BeginOAuthLogin: func(string) (OAuthLogin, error) {
			t.Fatal("unconfigured native OAuth should not invoke the login factory")
			return nil, nil
		},
	})
	model = selectCloudModelCatalog(t, model, "copilot")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if command != nil {
		t.Fatal("unconfigured native OAuth should not start a login")
	}
	updated := next.(Model)
	if !strings.Contains(updated.commandOutput, "gator connect copilot") {
		t.Fatalf("login guidance = %q", updated.commandOutput)
	}
}

func TestModelCatalogRunsProviderOwnedLoginInTheTUIWhenAvailable(t *testing.T) {
	t.Setenv("GATOR_CODEX_OAUTH_CLIENT_ID", "")
	model := New(Config{
		Provider:    "codex",
		LocalModels: &fakeLocalModelManager{},
		NewConnectCommand: func(provider string) (*exec.Cmd, error) {
			if provider != "codex" {
				t.Fatalf("provider = %q", provider)
			}
			return exec.Command("true"), nil
		},
	})
	model = selectCloudModelCatalog(t, model, "codex")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if command == nil {
		t.Fatal("provider-owned login did not start a terminal command")
	}
	updated := next.(Model)
	if !strings.Contains(updated.notice.text, "Opening the provider-owned sign-in") {
		t.Fatalf("login notice = %q", updated.notice.text)
	}
}

func TestModelCatalogRunsClaudeConnectViaModel(t *testing.T) {
	connected := false
	model := New(Config{
		Provider:    "claude",
		LocalModels: &fakeLocalModelManager{},
		NewConnectCommand: func(provider string) (*exec.Cmd, error) {
			if provider != "claude" {
				t.Fatalf("provider = %q", provider)
			}
			connected = true
			return exec.Command("true"), nil
		},
	})
	model = selectCloudModelCatalog(t, model, "claude")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if command == nil || !connected {
		t.Fatalf("Claude connection command = %#v connected:%t", command, connected)
	}
	if !strings.Contains(next.(Model).notice.text, "provider-owned sign-in") {
		t.Fatalf("Claude connection notice = %q", next.(Model).notice.text)
	}
}

func TestProviderOwnedCodexLoginSelectsHarnessInsteadOfNativeOAuth(t *testing.T) {
	model := New(Config{
		Provider:     "codex",
		Model:        "gpt-5.6",
		Verification: [][]string{{"go", "test", "./..."}},
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{}, errors.New("Gator OAuth credential for \"codex\" is required")
		},
		NewDelegateCommand: func(string, string, string, [][]string, string) (DelegateCommand, error) {
			return DelegateCommand{Process: exec.Command("true")}, nil
		},
	})
	next, command := model.Update(connectDoneMsg{provider: "codex"})
	if command != nil {
		t.Fatal("completed login returned an unexpected command")
	}
	updated := next.(Model)
	if updated.delegateRuntime != "codex" || !strings.Contains(updated.commandOutput, "Codex CLI harness is ready") {
		t.Fatalf("provider-owned login state = runtime:%q output:%q", updated.delegateRuntime, updated.commandOutput)
	}
	if len(updated.preflight) != 1 || updated.preflight[0] != "describe a task" {
		t.Fatalf("harness preflight = %#v", updated.preflight)
	}
}

func TestProviderOwnedXAILoginSelectsOpenCodeHarness(t *testing.T) {
	model := New(Config{
		Provider: "xai",
		NewDelegateCommand: func(string, string, string, [][]string, string) (DelegateCommand, error) {
			return DelegateCommand{Process: exec.Command("true")}, nil
		},
	})
	next, command := model.Update(connectDoneMsg{provider: "xai"})
	if command != nil {
		t.Fatal("completed xAI login returned an unexpected command")
	}
	updated := next.(Model)
	if updated.delegateRuntime != "opencode" || !strings.Contains(updated.commandOutput, "OpenCode CLI harness is ready") {
		t.Fatalf("xAI connection state = runtime:%q output:%q", updated.delegateRuntime, updated.commandOutput)
	}
}

func TestProviderOwnedClaudeConnectSelectsClaudeHarness(t *testing.T) {
	model := New(Config{
		Provider:     "claude",
		Verification: [][]string{{"go", "test", "./..."}},
		NewDelegateCommand: func(string, string, string, [][]string, string) (DelegateCommand, error) {
			return DelegateCommand{Process: exec.Command("true")}, nil
		},
	})
	next, command := model.Update(connectDoneMsg{provider: "claude"})
	if command != nil {
		t.Fatal("completed Claude connection returned an unexpected command")
	}
	updated := next.(Model)
	if updated.delegateRuntime != "claude" || !strings.Contains(updated.commandOutput, "Claude Code harness is ready") {
		t.Fatalf("Claude connection state = runtime:%q output:%q", updated.delegateRuntime, updated.commandOutput)
	}
}

func TestOpenCodeManagementSelectsTheVendorHarnessWithoutReadingCredentials(t *testing.T) {
	var received []string
	model := New(Config{
		NewOpenCodeCommand: func(arguments []string) (*exec.Cmd, error) {
			received = append([]string(nil), arguments...)
			return exec.Command("true"), nil
		},
	})
	model.task.SetValue("/opencode login xai")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil || !reflect.DeepEqual(received, []string{"login", "--provider", "xai"}) {
		t.Fatalf("OpenCode login command = %#v args=%#v", command, received)
	}
	updated, command := next.(Model).Update(openCodeCommandDoneMsg{action: "login", provider: "xai"})
	if command == nil {
		t.Fatal("OpenCode login did not return focus command")
	}
	model = updated.(Model)
	if model.delegateRuntime != "opencode" || model.provider.Value() != "opencode" || !strings.Contains(model.commandOutput, "OpenCode login completed") {
		t.Fatalf("OpenCode login state = runtime:%q provider:%q output:%q", model.delegateRuntime, model.provider.Value(), model.commandOutput)
	}
	model.task.SetValue("/opencode use xai/grok-build")
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("OpenCode selection did not focus composer")
	}
	model = next.(Model)
	if model.delegateRuntime != "opencode" || model.provider.Value() != "opencode" || model.model.Value() != "xai/grok-build" {
		t.Fatalf("OpenCode selection = runtime:%q provider:%q model:%q", model.delegateRuntime, model.provider.Value(), model.model.Value())
	}
}

func TestVersionAndCheckOnlyUpdateStatusStayInsideTheTUI(t *testing.T) {
	model := New(Config{
		Build: BuildInfo{Version: "v1.2.3", Commit: "abc123", Date: "2026-08-27"},
		CheckForUpdate: func() (UpdateStatus, error) {
			return UpdateStatus{Current: "v1.2.3", Latest: "v1.2.4", Available: true}, nil
		},
	})
	model.task.SetValue("/version")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil || !strings.Contains(next.(Model).commandOutput, "v1.2.3") || !strings.Contains(next.(Model).commandOutput, "abc123") {
		t.Fatalf("version output = %q command=%v", next.(Model).commandOutput, command)
	}
	model = next.(Model)
	model.task.SetValue("/update")
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil || !next.(Model).updateChecking {
		t.Fatalf("update check = command:%v checking:%t", command, next.(Model).updateChecking)
	}
	updated, follow := next.(Model).Update(command())
	if follow != nil {
		t.Fatal("update result returned an unexpected command")
	}
	model = updated.(Model)
	if model.updateChecking || !strings.Contains(model.commandOutput, "v1.2.4") || !strings.Contains(model.commandOutput, "gator update") {
		t.Fatalf("update output = checking:%t %q", model.updateChecking, model.commandOutput)
	}
}

func TestCodexHarnessRunDoesNotConstructNativeExecutor(t *testing.T) {
	delegateCalls := 0
	nativeCalls := 0
	model := New(Config{
		RepositoryPath: "/tmp/example-repository",
		Provider:       "codex",
		Model:          "gpt-5.6",
		Verification:   [][]string{{"go", "test", "./..."}},
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			nativeCalls++
			return gatorrun.Executor{}, errors.New("Gator OAuth credential for \"codex\" is required")
		},
		NewDelegateCommand: func(runtime, task, modelName string, verification [][]string, repository string) (DelegateCommand, error) {
			delegateCalls++
			if runtime != "codex" || task != "Inspect this repository" || modelName != "gpt-5.6" || repository != "/tmp/example-repository" {
				t.Fatalf("delegate input = runtime:%q task:%q model:%q repository:%q", runtime, task, modelName, repository)
			}
			if got := formatVerification(verification); got != "go test ./..." {
				t.Fatalf("delegate verification = %q", got)
			}
			return DelegateCommand{Process: exec.Command("true"), Output: func() string { return "delegated output" }}, nil
		},
	})
	model.delegateRuntime = "codex"
	model.task.SetValue("Inspect this repository")
	nativeCalls = 0
	model.refreshPreflight()
	if len(model.preflight) != 0 {
		t.Fatalf("harness preflight = %#v", model.preflight)
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if command == nil {
		t.Fatal("harness run did not start a terminal command")
	}
	if nativeCalls != 0 || delegateCalls != 1 || next.(Model).execution != nil {
		t.Fatalf("run dispatch = native:%d delegate:%d execution:%#v", nativeCalls, delegateCalls, next.(Model).execution)
	}
	completed, completionCommand := next.(Model).Update(delegatedRunDoneMsg{runtime: "codex"})
	if completionCommand != nil {
		t.Fatal("harness completion returned an unexpected command")
	}
	finished := completed.(Model)
	if finished.notice.kind != noticeSuccess || !strings.Contains(finished.commandOutput, "Codex CLI harness completed") {
		t.Fatalf("harness completion = notice:%#v output:%q", finished.notice, finished.commandOutput)
	}
}

func TestDelegatedRunFailureShowsCapturedTerminalOutput(t *testing.T) {
	model := New(Config{})
	next, command := model.Update(delegatedRunDoneMsg{runtime: "codex", output: "error: invalid Codex command", err: errors.New("exit status 2")})
	if command != nil {
		t.Fatal("delegated failure returned an unexpected command")
	}
	updated := next.(Model)
	if updated.notice.kind != noticeError || !strings.Contains(updated.commandOutput, "error: invalid Codex command") {
		t.Fatalf("delegated failure = notice:%#v output:%q", updated.notice, updated.commandOutput)
	}
}

func TestProviderDropdownDescribesNewDirectProviderChoices(t *testing.T) {
	model := New(Config{StateDir: t.TempDir()})
	model.focus = providerField
	want := map[string]string{
		"azure-openai-responses": "Responses API",
		"minimax-cn":             "Messages API",
		"zai-coding-cn":          "China",
		"opencode":               "OpenCode Zen",
		"opencode-go":            "OpenCode Go",
	}
	for _, option := range model.dropdownOptions() {
		fragment, ok := want[option.value]
		if !ok {
			continue
		}
		if !strings.Contains(option.description, fragment) {
			t.Fatalf("provider %q description = %q; want %q", option.value, option.description, fragment)
		}
		delete(want, option.value)
	}
	if len(want) != 0 {
		t.Fatalf("provider options missing: %#v", want)
	}
}

func TestModelDropdownKeepsCustomModelEntryForEndpointSpecificProviders(t *testing.T) {
	model := New(Config{Provider: "azure-openai"})
	model.focus = modelField
	options := model.dropdownOptions()
	if len(options) != 1 || !options[0].custom || options[0].label != "custom model ID" {
		t.Fatalf("Azure model options = %#v", options)
	}
}

func TestCustomModelDropdownDoesNotClearTypedModel(t *testing.T) {
	model := New(Config{Provider: "azure-openai", Model: "my-deployment"})
	model.focus = modelField
	model.normalizeDropdownSelection()
	model.applySelectedDropdown()
	if got := model.model.Value(); got != "my-deployment" {
		t.Fatalf("custom model = %q", got)
	}
}
