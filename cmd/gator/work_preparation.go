package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/worksession"
)

// This runs before Work classification/model calls or constructing its service. It can
// attest an unstarted preparation; failures after dispatch never use this path.
func sourcePreparationFailure(stateDir, source, conversation, prompt string, options workapp.RunOptions) *workapp.RunResult {
	if options.RuntimeRequestID == "" || len(options.ConnectorIDs) == 0 {
		return nil
	}
	settings, err := loadSettings()
	if err != nil {
		return nil
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return nil
	}
	missing := []string{}
	for _, id := range options.ConnectorIDs {
		if _, ok := registry.Get(id); !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	failure := workapp.RunResult{Error: "Selected source configuration is missing: " + strings.Join(missing, ", ")}
	store, err := worksession.Open(stateDir)
	if err != nil {
		failure.Error = err.Error()
		return &failure
	}
	raw, err := json.Marshal(options)
	if err != nil {
		failure.Error = err.Error()
		return &failure
	}
	boundary, err := store.PreparationBoundary(conversation, options.RuntimeRequestID, prompt, source, raw)
	if err != nil {
		failure.Error = fmt.Sprintf("could not verify original preparation: %v", err)
		return &failure
	}
	scope, err := captureConversationSetup(workapp.ConversationInput{Version: 1, Source: source, RunOptions: options}, true)
	if err != nil {
		failure.Error = err.Error()
		return &failure
	}
	if options.DialogueParent != "" || options.DialogueTurn != "" {
		original, _, err := resolveDialogueExecutionForPreparation(stateDir, source, conversation, prompt, options)
		if err != nil {
			failure.Error = err.Error()
			return &failure
		}
		if original != nil {
			scope.ScopeDigest, scope.ProcessingDigest = original.ScopeDigest, original.ProcessingDigest
		}
	}
	recovery := &worksession.RequestRecovery{Preparation: &boundary, Kind: "source_access", ScopeDigest: scope.ScopeDigest, ProcessingDigest: scope.ProcessingDigest, RetrySafe: true, Summary: "This attempt was rejected before Work execution because a selected source is no longer configured.", Repair: "Repair the original source or explicitly choose a replacement account. Review the exact scope and a fresh plan before starting the retained task."}
	for _, id := range missing {
		recovery.Needs = append(recovery.Needs, worksession.TaskAccessNeed{ConnectorID: id, Kind: "unconfigured", Operation: "configure"})
	}
	failure.Recovery = recovery
	return &failure
}
