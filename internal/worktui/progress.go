package worktui

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
