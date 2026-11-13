package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/goal"
	sharedusage "github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

// Capture account and capability configuration locally, without sending another
// account probe or recording credentials in a conversation. The existing runtime
// also enforces the captured OAuth grant pins when each connector is used.
func captureConversationInput(input workapp.ConversationInput) (workapp.ConversationInput, error) {
	settings, err := loadSettings()
	if err != nil {
		return input, err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return input, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return input, err
	}
	selected := []connector.Descriptor{}
	grants := map[string]string{}
	pins := map[string]string{}
	credentialPins := map[string]string{}
	for _, id := range input.RunOptions.ConnectorIDs {
		descriptor, ok := registry.Get(id)
		if !ok {
			return input, fmt.Errorf("selected source %q is no longer configured", id)
		}
		selected = append(selected, descriptor)
		credential, found, err := credentials.Read(descriptor.CredentialRef())
		if err != nil {
			return input, err
		}
		if !found && descriptor.Authentication == connector.AuthNone {
			credentialPins[id] = connector.CredentialPin(descriptor, auth.Credential{})
		}
		if found {
			credentialPins[id] = connector.CredentialPin(descriptor, credential)
			data, err := json.Marshal(credential)
			if err != nil {
				return input, err
			}
			if descriptor.Authentication == connector.AuthOAuth && credential.Refresh != "" {
				pins[id] = connector.OAuthGrantPin(descriptor, credential.Refresh)
				grants[id] = pins[id]
			} else {
				grants[id] = fmt.Sprintf("%x", sha256.Sum256(data))
			}
		}
	}
	// These settings can change execution authority or processing destination.
	// Cosmetic preferences/default-model changes do not invalidate captured work.
	scope := map[string]any{"connectors": selected, "grants": grants, "permissions": settings.ConnectorPermissions, "endpoints": settings.ProviderEndpoints, "provider_options": settings.ProviderOptions, "custom_providers": settings.CustomProviders, "extensions": settings.Extensions, "extension_trusts": settings.ExtensionTrusts, "hooks": settings.HookTrusts, "lsp": settings.LSPTrusts, "mcp": settings.MCPTrusts, "execution": settings.Execution, "roles": settings.WorkRoles, "external_agents": settings.ExternalAgents}
	data, err := json.Marshal(scope)
	if err != nil {
		return input, err
	}
	processing := map[string]any{}
	for key, value := range scope {
		if key != "connectors" && key != "grants" && key != "permissions" {
			processing[key] = value
		}
	}
	processingData, _ := json.Marshal(processing)
	input.ProcessingDigest = fmt.Sprintf("%x", sha256.Sum256(processingData))
	input.CredentialPins = credentialPins
	input.ScopeDigest = fmt.Sprintf("%x", sha256.Sum256(data))
	input.OAuthRefreshPins = pins
	return input, nil
}

func verifyConversationScope(input workapp.ConversationInput) error {
	current, err := captureConversationInput(input)
	if err != nil {
		return err
	}
	if input.ScopeDigest == "" || input.ScopeDigest != current.ScopeDigest || !maps.Equal(input.OAuthRefreshPins, current.OAuthRefreshPins) {
		return errors.New("accounts, permissions or processing configuration changed after this message; review the retained task and send a new request")
	}
	return nil
}

// Both planning and execution receive the same admitted public references.
// Past task controls, tool replay and approval tokens are never inherited.
func dialogueReferenceContext(input *workapp.ConversationInput) (string, error) {
	if input == nil {
		return "", nil
	}
	tasks := append([]workrun.ConversationTask(nil), input.TaskContext...)
	for index := range tasks {
		tasks[index].CanGuide, tasks[index].CanStop = false, false
		tasks[index].CanRevisePlan = false
		tasks[index].CanReviewInputs = false
		tasks[index].CanCorrectAction = false
		tasks[index].ReviewControlID = ""
	}
	data, err := json.Marshal(struct {
		Messages []workrun.ConversationMessage `json:"messages,omitempty"`
		Tasks    []workrun.ConversationTask    `json:"tasks,omitempty"`
	}{input.Recent, tasks})
	return string(data), err
}

func routeInteractiveConversation(stateDir string) func(context.Context, workrun.ConversationRequest, workapp.ConversationInput) (workrun.ConversationDecision, error) {
	return func(ctx context.Context, request workrun.ConversationRequest, input workapp.ConversationInput) (workrun.ConversationDecision, error) {
		if err := verifyConversationScope(input); err != nil {
			return workrun.ConversationDecision{}, err
		}
		provider, model, err := resolveConfiguredProvider(input.RunOptions.ModelProvider, input.RunOptions.ModelName)
		if err != nil {
			return workrun.ConversationDecision{}, err
		}
		backend, err := nativeWorkModel(provider, model, "")
		if err != nil {
			return workrun.ConversationDecision{}, err
		}
		return workrun.RouteConversation(sharedusage.WithState(sharedusage.WithCategory(ctx, "routing"), stateDir), backend, request)
	}
}

// Execution uses the child's own lease/replay/approval owner. Parent context is
// a bounded public reference, never the parent's replay or authorization.
func resolveDialogueExecution(stateDir, source, conversation, text string, options workapp.RunOptions) (*workapp.ConversationInput, goal.Record, error) {
	if options.DialogueParent == "" && options.DialogueTurn == "" {
		return nil, goal.Record{}, nil
	}
	store, err := worksession.Open(stateDir)
	if err != nil {
		return nil, goal.Record{}, err
	}
	origin, err := store.DialogueTask(conversation)
	if err != nil {
		return nil, goal.Record{}, err
	}
	if origin == nil || origin.ParentConversationID != options.DialogueParent || origin.TurnID != options.DialogueTurn {
		return nil, goal.Record{}, errors.New("independent task origin changed")
	}
	turn, err := store.ReadDialogue(options.DialogueParent, options.DialogueTurn)
	if err != nil {
		return nil, goal.Record{}, err
	}
	var input workapp.ConversationInput
	if json.Unmarshal(turn.Options, &input) != nil || input.Version != 1 || input.Source != source || turn.Text != text || turn.TaskChat != conversation {
		return nil, goal.Record{}, errors.New("independent task inputs changed")
	}
	expected := input.RunOptions
	expected.DialogueParent, expected.DialogueTurn = options.DialogueParent, options.DialogueTurn
	a, err := json.Marshal(expected)
	if err != nil {
		return nil, goal.Record{}, err
	}
	b, err := json.Marshal(options)
	if err != nil {
		return nil, goal.Record{}, err
	}
	if string(a) != string(b) {
		journal, err := store.LoadRequests(conversation)
		if err != nil {
			return nil, goal.Record{}, err
		}
		matched := false
		for _, retained := range journal.Requests {
			if retained.ClientID == origin.TurnID && retained.Text == text && worksession.MatchesPlanInputEvolution(retained, a, b) {
				matched = true
			}
		}
		if !matched {
			return nil, goal.Record{}, errors.New("independent task execution settings changed")
		}
	}
	// A source/account transition is authority only when retained in the
	// original request's digest-bound scope chain after explicit human review.
	journal, err := store.LoadRequests(conversation)
	if err != nil {
		return nil, goal.Record{}, err
	}
	for _, retained := range journal.Requests {
		if retained.ClientID != origin.TurnID || retained.Text != text || !worksession.MatchesPlanInputEvolution(retained, a, b) {
			continue
		}
		if access := retained.Plan.LatestAccessChange(); access != nil {
			first := ""
			for _, change := range retained.Plan.InputChanges {
				if change.Access != nil {
					first = change.Access.BeforeScope
					break
				}
			}
			if first != input.ScopeDigest || input.ProcessingDigest == "" || access.ProcessingDigest != input.ProcessingDigest {
				return nil, goal.Record{}, errors.New("reviewed account access does not match the original task")
			}
			input.RunOptions = options
			input.ScopeDigest = access.AfterScope
			input.OAuthRefreshPins = access.OAuthPins
			input.CredentialPins = access.CredentialPins
		}
	}
	if err := verifyConversationScope(input); err != nil {
		return nil, goal.Record{}, err
	}
	var record goal.Record
	if input.GoalID != "" {
		goals, err := goal.Open(stateDir)
		if err != nil {
			return nil, record, err
		}
		record, err = goals.Load(input.GoalID)
		if err != nil {
			return nil, record, err
		}
		if record.Status != goal.Active || record.SourcePath != source || record.ConversationID != options.DialogueParent {
			return nil, record, errors.New("the original goal or its workspace changed; inspect it before starting new work")
		}
	}
	return &input, record, nil
}
