package workapp

import (
	"github.com/gongahkia/gator/internal/agent"
	"strings"
)

func workProgress(event agent.Event) string {
	switch event.Kind {
	case agent.EventTextDelta, agent.EventText:
		return "Preparing your response"
	case agent.EventToolCalled:
		if event.ToolCall != nil {
			switch name := event.ToolCall.Name; {
			case strings.Contains(name, "read"), strings.Contains(name, "search"), strings.Contains(name, "list"):
				return "Reviewing selected sources"
			case strings.Contains(name, "artifact"), strings.Contains(name, "write"):
				return "Preparing a draft for review"
			case strings.Contains(name, "verify"), strings.Contains(name, "status"):
				return "Checking the result against evidence"
			}
		}
	case agent.EventToolFinished:
		if event.ToolError != "" {
			return "Reviewing a blocker; progress retained"
		}
		return "Checking the result"
	case agent.EventCompletionBlocked:
		return "Checking required output before finishing"
	case agent.EventCommandApprovalRequested:
		return "Waiting for your approval"
	case agent.EventSteeringApplied:
		return "Applying your updated direction"
	case agent.EventContextCompacted:
		return "Continuing with retained task context"
	case agent.EventRunFinished:
		return "Finishing and retaining results"
	}
	return "Working toward your requested outcome"
}

func specialistProgress(role, state string) string {
	if state == "completed" {
		return "Checking results and preparing the next step"
	}
	if state == "failed" {
		return "Reviewing a blocker; progress retained"
	}
	switch role {
	case "source_researcher", "connected_researcher":
		return "Reviewing selected sources"
	case "artifact_reviewer", "claim_verifier":
		return "Checking the draft against evidence"
	case "goal_reviewer":
		return "Comparing changes with your goal"
	case "spreadsheet_analyst":
		return "Checking the numbers"
	case "code":
		return "Preparing a change for review"
	default:
		return "Working toward your requested outcome"
	}
}
