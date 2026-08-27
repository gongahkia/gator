package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
	modelprovider "github.com/gongahkia/gator/internal/model"
)

func TestAzureCloudEndpointExpandsResourceURLs(t *testing.T) {
	chatEndpoint, err := azureCloudEndpoint(modelprovider.AzureOpenAI, "example-resource.openai.azure.com", "review", "2025-04-01-preview")
	if err != nil {
		t.Fatalf("expand Azure Chat Completions endpoint: %v", err)
	}
	if want := "https://example-resource.openai.azure.com/openai/deployments/review/chat/completions?api-version=2025-04-01-preview"; chatEndpoint != want {
		t.Fatalf("Azure Chat Completions endpoint = %q, want %q", chatEndpoint, want)
	}

	responsesEndpoint, err := azureCloudEndpoint(modelprovider.AzureOpenAIResponses, "https://example-resource.openai.azure.com", "review", "")
	if err != nil {
		t.Fatalf("expand Azure Responses endpoint: %v", err)
	}
	if want := "https://example-resource.openai.azure.com/openai/v1/responses?api-version=v1"; responsesEndpoint != want {
		t.Fatalf("Azure Responses endpoint = %q, want %q", responsesEndpoint, want)
	}
}

func TestCloudModelSetupMasksCredentialAndUpdatesSelectedModel(t *testing.T) {
	var saved CloudModelSetup
	model := New(Config{
		ProviderEndpoints: map[string]string{"azure-openai": "https://existing.openai.azure.com"},
		SaveCloudModel: func(setup CloudModelSetup) error {
			saved = setup
			return nil
		},
	})
	model.width, model.height = 100, 42
	model.screen = localModelsScreen
	form := newCloudModelSetupForm(modelprovider.AzureOpenAI, "review", "", nil, model.inlineWidth())
	form.credential.SetValue("masked-api-key")
	form.endpoint.SetValue("example-resource.openai.azure.com")
	form.apiVersion.SetValue("2025-04-01-preview")
	form.focus = cloudSetupAPIVersionField
	model.localModels.cloudSetup = &form

	if view := model.cloudModelSetupView(); strings.Contains(view, "masked-api-key") {
		t.Fatal("cloud setup view exposed the typed credential")
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("save did not schedule the configuration callback")
	}
	message := command()
	updated, _ := next.(Model).Update(message)
	model = updated.(Model)
	if saved.APIKey != "masked-api-key" || saved.Model != "review" {
		t.Fatal("cloud configuration callback did not receive the expected setup")
	}
	if model.localModels.cloudSetup != nil || model.provider.Value() != "azure-openai" || model.model.Value() != "review" {
		t.Fatalf("saved TUI selection = form:%#v provider:%q model:%q", model.localModels.cloudSetup, model.provider.Value(), model.model.Value())
	}
	if strings.Contains(model.View(), "masked-api-key") {
		t.Fatal("saved TUI view exposed the typed credential")
	}
}

func TestModelCatalogOpensAzureConfigurationForm(t *testing.T) {
	model := New(Config{
		LocalModels:       &fakeLocalModelManager{},
		ProviderEndpoints: map[string]string{"azure-openai": "https://existing.openai.azure.com"},
		SaveCloudModel:    func(CloudModelSetup) error { return nil },
	})
	model.width, model.height = 100, 42
	model = selectCloudModelCatalog(t, model, "azure-openai")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if command == nil {
		t.Fatal("Azure configuration did not focus the first form field")
	}
	configured := next.(Model)
	if configured.localModels.cloudSetup == nil {
		t.Fatal("Azure configuration form did not open")
	}
	if got := configured.localModels.cloudSetup.endpoint.Value(); got != "https://existing.openai.azure.com" {
		t.Fatalf("pre-filled Azure endpoint = %q", got)
	}
	view := configured.View()
	if !strings.Contains(view, "Deployment or model") || !strings.Contains(view, "Azure resource URL or endpoint") || !strings.Contains(view, "API key") {
		t.Fatalf("Azure configuration view = %q", view)
	}
}

func TestCloudModelSetupSupportsEveryCredentialAndMetadataFamily(t *testing.T) {
	tests := []struct {
		name      string
		provider  modelprovider.Provider
		configure func(*cloudModelSetupForm)
		check     func(*testing.T, CloudModelSetup)
	}{
		{
			name:     "Claude Code Anthropic API key",
			provider: modelprovider.Claude,
			configure: func(form *cloudModelSetupForm) {
				form.model.SetValue("claude-sonnet-5")
				form.credential.SetValue("claude-key")
			},
			check: func(t *testing.T, setup CloudModelSetup) {
				if setup.DelegateRuntime != "claude" || setup.CredentialType != cloudCredentialAPIKey {
					t.Fatalf("Claude setup = %#v", setup)
				}
			},
		},
		{
			name:     "Amazon Bedrock bearer and region",
			provider: modelprovider.AmazonBedrock,
			configure: func(form *cloudModelSetupForm) {
				form.model.SetValue("openai.gpt-oss-20b-1:0")
				form.credential.SetValue("bedrock-token")
				form.region.SetValue("eu-west-1")
				form.awsProfile.SetValue("engineering")
			},
			check: func(t *testing.T, setup CloudModelSetup) {
				if setup.CredentialType != cloudCredentialBearer || setup.Options["region"] != "eu-west-1" || setup.Options["profile"] != "engineering" {
					t.Fatalf("Bedrock setup = %#v", setup)
				}
			},
		},
		{
			name:     "Google Vertex access token project and location",
			provider: modelprovider.GoogleVertex,
			configure: func(form *cloudModelSetupForm) {
				form.model.SetValue("google/gemini-2.0-flash-001")
				form.credential.SetValue("vertex-token")
				form.project.SetValue("project-123")
				form.location.SetValue("us-central1")
				form.credentialsPath.SetValue("/tmp/gator-adc.json")
			},
			check: func(t *testing.T, setup CloudModelSetup) {
				if setup.CredentialType != cloudCredentialBearer || setup.Options["project"] != "project-123" || setup.Options["location"] != "us-central1" || setup.Options["credentials_path"] != "/tmp/gator-adc.json" {
					t.Fatalf("Vertex setup = %#v", setup)
				}
			},
		},
		{
			name:     "Cloudflare AI Gateway metadata",
			provider: modelprovider.CloudflareGateway,
			configure: func(form *cloudModelSetupForm) {
				form.model.SetValue("openai/gpt-5.6")
				form.credential.SetValue("cloudflare-token")
				form.accountID.SetValue("account-123")
				form.gatewayID.SetValue("gateway-123")
				form.gatewayProtocol.SetValue("openai-responses")
			},
			check: func(t *testing.T, setup CloudModelSetup) {
				if setup.Options["account_id"] != "account-123" || setup.Options["gateway_id"] != "gateway-123" || setup.Options["gateway_protocol"] != "openai-responses" {
					t.Fatalf("Cloudflare Gateway setup = %#v", setup)
				}
			},
		},
		{
			name:     "Codex account OAuth",
			provider: modelprovider.Codex,
			configure: func(form *cloudModelSetupForm) {
				form.model.SetValue("gpt-5.6")
			},
			check: func(t *testing.T, setup CloudModelSetup) {
				if setup.APIKey != "" || setup.CredentialType != "" {
					t.Fatalf("Codex setup unexpectedly accepted a credential: %#v", setup)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			form := newCloudModelSetupForm(test.provider, "", "", nil, 100)
			test.configure(&form)
			setup, err := form.configuration()
			if err != nil {
				t.Fatalf("configuration: %v", err)
			}
			test.check(t, setup)
		})
	}
}

func TestEveryDirectCloudProviderOffersTUIConfiguration(t *testing.T) {
	for _, providerName := range modelprovider.Names() {
		t.Run(providerName, func(t *testing.T) {
			provider, err := modelprovider.ParseProvider(providerName)
			if err != nil {
				t.Fatalf("parse provider: %v", err)
			}
			endpoint := ""
			if provider == modelprovider.AzureOpenAI || provider == modelprovider.AzureOpenAIResponses {
				endpoint = "https://example-resource.openai.azure.com"
			}
			form := newCloudModelSetupForm(provider, "test-model", endpoint, nil, 100)
			if len(form.fields()) == 0 {
				t.Fatal("provider has no TUI configuration fields")
			}
			if _, err := form.configuration(); err != nil {
				t.Fatalf("provider configuration form is unavailable: %v", err)
			}
		})
	}
}

func TestLocalModelsAreAvailableThroughTheTUIAndConfigureTheNextRun(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:     "http://127.0.0.1:11434",
		RuntimeVersion: "test",
		Executable:     "/usr/bin/ollama",
		Models: []LocalModel{{
			ID:          "qwen2.5-coder-7b",
			OllamaModel: "qwen2.5-coder:7b",
			Name:        "Qwen2.5-Coder 7B",
			DefaultName: "Qwen2.5-Coder 7B",
			Download:    "4.7 GB",
			Context:     "32K",
			Summary:     "smallest reviewed coding option",
			SourceURL:   "https://ollama.example/qwen",
		}},
	}}
	model := New(Config{RepositoryPath: t.TempDir(), LocalModels: manager})
	model.width, model.height = 100, 42
	model.resizeInputs()
	model.task.SetValue("/model")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("/model did not start a status refresh")
	}
	model = next.(Model)
	if model.screen != localModelsScreen || model.localModels.action != localModelRefreshing {
		t.Fatalf("/model screen state = screen:%v action:%v", model.screen, model.localModels.action)
	}

	next, _ = model.Update(localModelStatusMsg{generation: model.localModels.generation, catalog: manager.catalog})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = next.(Model)
	if !strings.Contains(model.View(), "Qwen2.5-Coder 7B") || !strings.Contains(model.View(), "Connected · Ollama test") {
		t.Fatalf("local model view = %q", model.View())
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	model = next.(Model)
	if model.localModels.renaming == nil {
		t.Fatal("rename did not open a display-name editor")
	}
	model.localModels.renaming.input.SetValue("desk Qwen")
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = finishLocalModelOperation(t, next.(Model))
	if got := model.localModels.catalog.Models[0].Name; got != "desk Qwen" || model.config.ModelAliases[config.ModelAliasKey("gator-local", "qwen2.5-coder:7b")] != "desk Qwen" {
		t.Fatalf("renamed local model = %#v aliases:%#v", model.localModels.catalog.Models[0], model.config.ModelAliases)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmPull {
		t.Fatalf("pull confirmation = %v", model.localModels.confirmation)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = next.(Model)
	if model.localModels.action != localModelPulling || model.localModels.operation == nil {
		t.Fatalf("pull state = action:%v operation:%#v", model.localModels.action, model.localModels.operation)
	}
	model = finishLocalModelOperation(t, model)
	if !manager.pulled || !model.localModels.catalog.Models[0].Installed {
		t.Fatalf("pull state = manager:%#v catalog:%#v", manager, model.localModels.catalog)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.localModels.action != localModelUsing || model.localModels.operation == nil {
		t.Fatalf("use state = action:%v operation:%#v", model.localModels.action, model.localModels.operation)
	}
	model = finishLocalModelOperation(t, model)
	if !manager.used || model.provider.Value() != "gator-local" || model.model.Value() != "qwen2.5-coder:7b" {
		t.Fatalf("local selection = used:%t provider:%q model:%q", manager.used, model.provider.Value(), model.model.Value())
	}
	if provider, modelName, custom, err := model.resolveProviderAndModel(model.provider.Value(), model.model.Value()); err != nil || !custom || provider != "gator-local" || modelName != "qwen2.5-coder:7b" {
		t.Fatalf("selected local runtime = provider:%q model:%q custom:%t err:%v", provider, modelName, custom, err)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmRemove {
		t.Fatalf("remove confirmation = %v", model.localModels.confirmation)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = next.(Model)
	model = finishLocalModelOperation(t, model)
	if !manager.removed || model.provider.Value() != "" || model.model.Value() != "" {
		t.Fatalf("remove state = removed:%t provider:%q model:%q", manager.removed, model.provider.Value(), model.model.Value())
	}
}

func TestLocalModelsShowsUnavailableRuntimeWithoutHidingCatalog(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:   "http://127.0.0.1:11434",
		RuntimeError: "connect to local Ollama runtime: connection refused",
		Executable:   "/usr/bin/ollama",
		Models:       []LocalModel{{ID: "qwen2.5-coder-7b", Name: "Qwen2.5-Coder 7B", Download: "4.7 GB", Context: "32K"}},
	}}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	model.screen = localModelsScreen
	model.localModels.action = localModelRefreshing
	model.localModels.generation = 1
	next, _ := model.Update(localModelStatusMsg{generation: 1, catalog: manager.catalog})
	updated := next.(Model)
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated = next.(Model)
	view := updated.View()
	if updated.localModels.confirmation != localModelConfirmStart || !strings.Contains(view, "Unavailable") || !strings.Contains(view, "Start Ollama?") || !strings.Contains(view, "Qwen2.5-Coder 7B") {
		t.Fatalf("unavailable local model view = %q", view)
	}
	next, command := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if command != nil || next.(Model).localModels.confirmation != localModelNoConfirmation || !next.(Model).localModels.startDismissed || !strings.Contains(next.(Model).notice.text, "Start Ollama yourself") {
		t.Fatalf("manual local runtime choice = confirmation:%v dismissed:%t notice:%q command:%#v", next.(Model).localModels.confirmation, next.(Model).localModels.startDismissed, next.(Model).notice.text, command)
	}
}

func TestLocalModelsStartOllamaWithGatorByDefault(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:   "http://127.0.0.1:11434",
		RuntimeError: "connect to local Ollama runtime: connection refused",
		Executable:   "/usr/bin/ollama",
		Models:       []LocalModel{{ID: "qwen2.5-coder-7b", Name: "Qwen2.5-Coder 7B"}},
	}}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	model.screen = localModelsScreen
	model.localModels.action = localModelRefreshing
	model.localModels.generation = 1
	next, _ := model.Update(localModelStatusMsg{generation: 1, catalog: manager.catalog})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmStart {
		t.Fatalf("start prompt = %#v", model.localModels)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.localModels.action != localModelStarting || model.localModels.operation == nil {
		t.Fatalf("start operation = %#v", model.localModels)
	}
	model = finishLocalModelOperation(t, model)
	if !manager.started || model.localModels.catalog.RuntimeError != "" || !strings.Contains(model.notice.text, "running under this Gator session") {
		t.Fatalf("started local runtime = manager:%#v catalog:%#v notice:%q", manager, model.localModels.catalog, model.notice.text)
	}
}

func TestLocalModelsOfferInstallationHelpWhenOllamaIsMissing(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:   "http://127.0.0.1:11434",
		RuntimeError: "connect to local Ollama runtime: connection refused",
		Dependencies: []LocalDependency{
			{
				ID:           "ollama",
				Name:         "Ollama",
				Purpose:      "reviewed local coding models",
				HelpURL:      "https://ollama.com/download",
				Instructions: []string{"Install Ollama from its official download page, then refresh this screen."},
			},
			{
				ID:           "bubblewrap",
				Name:         "Bubblewrap",
				Purpose:      "strict Linux sandboxing",
				Required:     true,
				HelpURL:      "https://github.com/containers/bubblewrap",
				Instructions: []string{"Install the bubblewrap package."},
			},
		},
		Models: []LocalModel{{ID: "qwen2.5-coder-7b", Name: "Qwen2.5-Coder 7B"}},
	}}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	model.screen = localModelsScreen
	model.localModels.action = localModelRefreshing
	model.localModels.generation = 1
	next, _ := model.Update(localModelStatusMsg{generation: 1, catalog: manager.catalog})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmInstall || !strings.Contains(model.View(), "Install Ollama?") {
		t.Fatalf("install prompt = %#v view:%q", model.localModels, model.View())
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command != nil || manager.started || !model.localModels.dependencyHelp {
		t.Fatalf("installation help state = command:%#v manager:%#v state:%#v", command, manager, model.localModels)
	}
	view := model.View()
	if !strings.Contains(view, "https://ollama.com/download") || !strings.Contains(view, "Bubblewrap") || !strings.Contains(model.notice.text, "does not run system installers") {
		t.Fatalf("installation help view = %q", view)
	}
}

func TestLocalModelsDisableIneligibleCatalogEntriesBeforePullOrUse(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:     "http://127.0.0.1:11434",
		RuntimeVersion: "test",
		Models: []LocalModel{{
			ID:            "qwen2.5-coder-7b",
			OllamaModel:   "qwen2.5-coder:7b",
			Name:          "Qwen2.5-Coder 7B",
			Installed:     true,
			Requirement:   "needs 8.8 GiB RAM / 5.3 GiB disk",
			BlockedReason: "requires 8.8 GiB currently available RAM under Gator's guardrail; detected 6.0 GiB",
		}},
	}}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	model.screen = localModelsScreen
	model.localModels.section = localModelSection
	model.localModels.catalog = manager.catalog

	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	updated := next.(Model)
	if command != nil || updated.localModels.confirmation != localModelNoConfirmation || !strings.Contains(updated.notice.text, "disabled on this host") || manager.pulled {
		t.Fatalf("blocked pull = %#v command:%#v manager:%#v", updated, command, manager)
	}
	next, command = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = next.(Model)
	if command != nil || updated.localModels.operation != nil || !strings.Contains(updated.notice.text, "disabled on this host") || manager.used {
		t.Fatalf("blocked use = %#v command:%#v manager:%#v", updated, command, manager)
	}
	if view := updated.View(); !strings.Contains(view, "disabled") || !strings.Contains(view, "needs 8.8 GiB RAM") {
		t.Fatalf("blocked local model view = %q", view)
	}
}

func TestModelCatalogShowsCloudReadinessAndSelectsCloudModel(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	manager := &fakeLocalModelManager{}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	next, _ := model.openModelCatalog()
	model = next.(Model)
	for index, entry := range model.cloudModels() {
		if entry.provider == "openai" {
			model.localModels.cloudIndex = index
			break
		}
	}
	if !strings.Contains(model.View(), "API key set: OPENAI_API_KEY") {
		t.Fatalf("cloud readiness view = %q", model.View())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.provider.Value() != "openai" || model.model.Value() == "" {
		t.Fatalf("cloud selection = provider:%q model:%q", model.provider.Value(), model.model.Value())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	model = next.(Model)
	if model.localModels.renaming == nil {
		t.Fatal("cloud rename did not open a display-name editor")
	}
	model.localModels.renaming.input.SetValue("work OpenAI")
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = finishLocalModelOperation(t, next.(Model))
	if model.modelAlias("openai", model.model.Value()) != "work OpenAI" || !strings.Contains(model.View(), "work OpenAI") {
		t.Fatalf("cloud alias = %#v view:%q", model.config.ModelAliases, model.View())
	}
}

func TestModelCatalogExposesCheckedInOpenCodeModelsAndClaudeHarness(t *testing.T) {
	model := New(Config{LocalModels: &fakeLocalModelManager{}})
	next, _ := model.openModelCatalog()
	model = next.(Model)
	model.localModels.section = cloudModelSection
	entries := model.cloudModels()
	openCodeIndex := -1
	claudeIndex := -1
	for index, entry := range entries {
		if entry.provider == "opencode" && entry.model == "claude-opus-5" && entry.selectable {
			openCodeIndex = index
		}
		if entry.provider == "claude" && entry.name == "Claude Code · API-key harness" && entry.canLogin && !entry.selectable {
			claudeIndex = index
		}
	}
	if openCodeIndex < 0 || claudeIndex < 0 {
		t.Fatalf("cloud catalog missing entries: OpenCode=%d Claude=%d entries=%#v", openCodeIndex, claudeIndex, entries)
	}
	model.localModels.cloudIndex = openCodeIndex
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.provider.Value() != "opencode" || model.model.Value() != "claude-opus-5" {
		t.Fatalf("OpenCode catalog selection = provider:%q model:%q", model.provider.Value(), model.model.Value())
	}
}

func finishLocalModelOperation(t *testing.T, model Model) Model {
	t.Helper()
	for range 8 {
		if model.localModels.operation == nil {
			return model
		}
		message := waitForLocalModelOperation(model.localModels.operation)()
		next, _ := model.Update(message)
		model = next.(Model)
	}
	t.Fatalf("local model operation did not finish: %#v", model.localModels)
	return Model{}
}

func TestCloudCredentialRemovalRequiresConfirmation(t *testing.T) {
	backend := &fakeModelManagement{
		credentials: []StoredCredentialStatus{{Provider: "openai", StoreKey: "openai", Present: true, Kind: "API key"}},
	}
	model := New(Config{LocalModels: &fakeLocalModelManager{}, ModelManagement: backend})
	model.width, model.height = 100, 42
	model = selectCloudModelCatalog(t, model, "openai")
	model.applyCredentialStatuses(backend.credentials)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmRemoveCredential {
		t.Fatalf("confirmation = %v", model.localModels.confirmation)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(Model)
	if model.localModels.confirmation != localModelNoConfirmation || backend.removed != "" {
		t.Fatalf("cancel mutated credentials: confirm=%v removed=%q", model.localModels.confirmation, backend.removed)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = next.(Model)
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("confirm did not schedule credential removal")
	}
	message := command()
	updated, _ := next.(Model).Update(message)
	model = updated.(Model)
	if backend.removed != "openai" {
		t.Fatalf("removed = %q", backend.removed)
	}
	if strings.Contains(model.notice.text, "logged out") || !strings.Contains(model.notice.text, "OPENAI_API_KEY") && !strings.Contains(model.notice.text, "were not changed") {
		t.Fatalf("notice = %q", model.notice.text)
	}
}

func TestCustomProviderEditorCreatesAndRemovesWithoutSecrets(t *testing.T) {
	backend := &fakeModelManagement{}
	model := New(Config{LocalModels: &fakeLocalModelManager{}, ModelManagement: backend})
	model.width, model.height = 100, 42
	model.screen = localModelsScreen
	model.localModels.section = cloudModelSection
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if command == nil {
		t.Fatal("create form did not focus")
	}
	model = next.(Model)
	if model.localModels.customSetup == nil {
		t.Fatal("custom provider form did not open")
	}
	form := model.localModels.customSetup
	form.id.SetValue("team-gateway")
	form.baseURL.SetValue("https://models.example.com/v1/chat/completions")
	form.apiKeyEnv.SetValue("TEAM_GATEWAY_API_KEY")
	form.models.SetValue("coding-large\ncoding-small")
	form.defaultModel.SetValue("coding-large")
	form.focus = customProviderDefaultField
	if view := model.customProviderSetupView(); strings.Contains(view, "sk-secret") {
		t.Fatal("custom provider view contained a secret")
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmSaveCustom || model.localModels.pendingCustom == nil {
		t.Fatalf("review confirmation missing: %#v", model.localModels)
	}
	if strings.Contains(model.View(), "sk-") {
		t.Fatal("review view leaked a secret")
	}
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("save did not schedule")
	}
	message := command()
	updated, _ := next.(Model).Update(message)
	model = updated.(Model)
	if len(backend.providers) != 1 || backend.providers[0].APIKeyEnv != "TEAM_GATEWAY_API_KEY" {
		t.Fatalf("saved providers = %#v", backend.providers)
	}
	if model.provider.Value() != "team-gateway" || model.config.BaseURL != "https://models.example.com/v1/chat/completions" {
		t.Fatalf("selection = provider %q url %q", model.provider.Value(), model.config.BaseURL)
	}

	model = selectCloudModelCatalog(t, model, "team-gateway")
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmRemoveCustom {
		t.Fatalf("remove confirmation = %v", model.localModels.confirmation)
	}
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	message = command()
	updated, _ = next.(Model).Update(message)
	model = updated.(Model)
	if len(backend.providers) != 0 {
		t.Fatalf("providers after remove = %#v", backend.providers)
	}
}

func TestCustomProviderDiscoveryPreviewRequiresConfirm(t *testing.T) {
	backend := &fakeModelManagement{
		providers: []config.CustomProvider{{
			ID: "team-gateway", BaseURL: "https://models.example.com/v1/chat/completions", Models: []string{"old"}, DefaultModel: "old",
		}},
		discovery: CustomProviderDiscovery{ID: "team-gateway", Models: []string{"alpha", "beta"}, DefaultModel: "alpha"},
	}
	model := New(Config{
		LocalModels:       &fakeLocalModelManager{},
		ModelManagement:   backend,
		CustomProviders:   backend.providers,
		Provider:          "team-gateway",
		Model:             "old",
	})
	model.width, model.height = 100, 42
	model = selectCloudModelCatalog(t, model, "team-gateway")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if command == nil {
		t.Fatal("discover did not start")
	}
	model = next.(Model)
	message := waitForTeaCommands(t, command)
	updated, _ := model.Update(message)
	model = updated.(Model)
	if model.localModels.confirmation != localModelConfirmApplyDiscovery || backend.applied {
		t.Fatalf("preview applied early: confirm=%v applied=%t", model.localModels.confirmation, backend.applied)
	}
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	message = command()
	updated, _ = next.(Model).Update(message)
	model = updated.(Model)
	if !backend.applied || strings.Join(model.config.CustomProviders[0].Models, ",") != "alpha,beta" {
		t.Fatalf("applied catalog = %#v", model.config.CustomProviders)
	}
}

func TestGatorLocalCustomProviderIsReadOnlyInCloudCatalog(t *testing.T) {
	model := New(Config{
		LocalModels: &fakeLocalModelManager{},
		ModelManagement: &fakeModelManagement{},
		CustomProviders: []config.CustomProvider{{
			ID: "gator-local", BaseURL: "http://127.0.0.1:11434/v1/chat/completions", Models: []string{"qwen2.5-coder:7b"}, DefaultModel: "qwen2.5-coder:7b",
		}},
	})
	model.width, model.height = 100, 42
	model = selectCloudModelCatalog(t, model, "gator-local")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	model = next.(Model)
	if model.localModels.customSetup != nil || model.localModels.cloudSetup != nil {
		t.Fatal("gator-local opened an editor")
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = next.(Model)
	if model.localModels.confirmation != localModelNoConfirmation {
		t.Fatal("gator-local removal was offered from Cloud")
	}
}

func TestSelectingCustomProviderUsesItsEndpoint(t *testing.T) {
	model := New(Config{
		LocalModels: &fakeLocalModelManager{},
		CustomProviders: []config.CustomProvider{{
			ID: "team-gateway", BaseURL: "https://models.example.com/v1/chat/completions", Models: []string{"coding-large"}, DefaultModel: "coding-large",
		}},
		BaseURL: "https://api.openai.com",
		Provider: "openai",
	})
	model.width, model.height = 100, 42
	model = selectCloudModelCatalog(t, model, "team-gateway")
	next, _ := model.useCloudModel(model.cloudModels()[model.localModels.cloudIndex])
	model = next.(Model)
	if model.config.BaseURL != "https://models.example.com/v1/chat/completions" {
		t.Fatalf("selected custom base URL = %q", model.config.BaseURL)
	}
}

func waitForTeaCommands(t *testing.T, command tea.Cmd) tea.Msg {
	t.Helper()
	if command == nil {
		t.Fatal("missing command")
	}
	return command()
}

type fakeModelManagement struct {
	credentials []StoredCredentialStatus
	providers   []config.CustomProvider
	discovery   CustomProviderDiscovery
	removed     string
	applied     bool
}

func (backend *fakeModelManagement) CredentialStatuses() ([]StoredCredentialStatus, error) {
	return append([]StoredCredentialStatus(nil), backend.credentials...), nil
}

func (backend *fakeModelManagement) RemoveCredential(provider string) (CredentialRemovalResult, error) {
	backend.removed = provider
	backend.credentials = nil
	return CredentialRemovalResult{Provider: provider, StoreKey: provider, Removed: true, Kind: "API key", RemainingSources: []string{"OPENAI_API_KEY"}}, nil
}

func (backend *fakeModelManagement) SaveCustomProvider(setup CustomProviderSetup) ([]config.CustomProvider, error) {
	provider := config.CustomProvider{ID: setup.ID, BaseURL: setup.BaseURL, APIKeyEnv: setup.APIKeyEnv, Models: setup.Models, DefaultModel: setup.DefaultModel}
	backend.providers = []config.CustomProvider{provider}
	return backend.providers, nil
}

func (backend *fakeModelManagement) RemoveCustomProvider(id string) ([]config.CustomProvider, error) {
	backend.providers = nil
	return nil, nil
}

func (backend *fakeModelManagement) DiscoverCustomProvider(id string) (CustomProviderDiscovery, error) {
	return backend.discovery, nil
}

func (backend *fakeModelManagement) ApplyCustomProviderDiscovery(id string, models []string) ([]config.CustomProvider, error) {
	backend.applied = true
	if len(backend.providers) == 0 {
		backend.providers = []config.CustomProvider{{ID: id, Models: models, DefaultModel: models[0]}}
	} else {
		backend.providers[0].Models = append([]string(nil), models...)
		backend.providers[0].DefaultModel = models[0]
	}
	return backend.providers, nil
}
	catalog LocalModelCatalog
	started bool
	pulled  bool
	used    bool
	removed bool
}

func (manager *fakeLocalModelManager) Status(context.Context) (LocalModelCatalog, error) {
	return manager.catalog, nil
}

func (manager *fakeLocalModelManager) Start(context.Context) (LocalModelCatalog, error) {
	manager.started = true
	manager.catalog.RuntimeError = ""
	if manager.catalog.RuntimeVersion == "" {
		manager.catalog.RuntimeVersion = "started"
	}
	return manager.catalog, nil
}

func (manager *fakeLocalModelManager) Pull(_ context.Context, id string, report func(LocalModelProgress)) (LocalModelCatalog, error) {
	if id != "qwen2.5-coder-7b" {
		return LocalModelCatalog{}, context.Canceled
	}
	if report != nil {
		report(LocalModelProgress{Status: "pulling layers", Completed: 5, Total: 10})
	}
	manager.pulled = true
	manager.catalog.Models[0].Installed = true
	return manager.catalog, nil
}

func (manager *fakeLocalModelManager) Use(_ context.Context, id string) (LocalModelUpdate, error) {
	if id != "qwen2.5-coder-7b" || !manager.catalog.Models[0].Installed {
		return LocalModelUpdate{}, context.Canceled
	}
	manager.used = true
	return LocalModelUpdate{
		Catalog: manager.catalog,
		CustomProviders: []config.CustomProvider{{
			ID: "gator-local", BaseURL: "http://127.0.0.1:11434/v1/chat/completions", Models: []string{"qwen2.5-coder:7b"}, DefaultModel: "qwen2.5-coder:7b",
		}},
		Provider: "gator-local",
		Model:    "qwen2.5-coder:7b",
	}, nil
}

func (manager *fakeLocalModelManager) Remove(_ context.Context, id string) (LocalModelUpdate, error) {
	if id != "qwen2.5-coder-7b" {
		return LocalModelUpdate{}, context.Canceled
	}
	manager.removed = true
	manager.catalog.Models[0].Installed = false
	return LocalModelUpdate{Catalog: manager.catalog}, nil
}

func (manager *fakeLocalModelManager) Rename(_ context.Context, provider, model, alias string) (map[string]string, error) {
	return map[string]string{config.ModelAliasKey(provider, model): alias}, nil
}
