package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workrun"
)

// These presenters read the same stores as the command actions. They run only
// after a successful action; script output remains owned by the CLI commands.
func workMemoryResult(stateDir, activeGoal string, arguments []string) (*uisnapshot.Result, error) {
	if len(arguments) == 0 {
		return nil, nil
	}
	store, err := memory.Open(stateDir)
	if err != nil {
		return nil, err
	}
	scope, remaining := uiScope(activeGoal, arguments[1:])
	switch arguments[0] {
	case "list", "search":
		var records []memory.Record
		if arguments[0] == "list" {
			records, err = store.List(scope, containsWord(remaining, "--all"))
		} else {
			records, err = store.Search(strings.Join(remaining, " "), scope, 30)
		}
		if err != nil {
			return nil, err
		}
		result := &uisnapshot.Result{Kind: "list", Title: "Memory", Summary: fmt.Sprintf("%d memories", len(records))}
		if len(records) == 0 {
			result.Summary = "No matching memories."
		}
		for _, record := range records {
			state := record.Kind
			if record.SourceStatus != "" {
				state += " · " + record.SourceStatus
			}
			result.Items = append(result.Items, uisnapshot.ResultItem{Label: record.Text, Detail: record.ID, State: state})
		}
		return result, nil
	case "show":
		if len(arguments) != 2 {
			return nil, nil
		}
		record, err := store.Load(arguments[1])
		if err != nil {
			return nil, err
		}
		return &uisnapshot.Result{Kind: "details", Title: "Memory", Summary: record.Text, Fields: []uisnapshot.Field{
			{Label: "ID", Value: record.ID}, {Label: "Kind", Value: record.Kind}, {Label: "Goal", Value: uiEmpty(record.GoalID)},
			{Label: "Source", Value: uiEmpty(record.Source)}, {Label: "Source status", Value: uiEmpty(record.SourceStatus)},
			{Label: "Expires", Value: formatMemoryExpiry(record.ExpiresAt)}, {Label: "Updated", Value: record.UpdatedAt.Local().Format("2006-01-02 15:04")},
		}}, nil
	}
	return nil, nil
}

func workNoteResult(activeGoal, command string) (*uisnapshot.Result, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, nil
	}
	store, _, err := knowledgeStore()
	if err != nil {
		return nil, err
	}
	switch parts[0] {
	case "list", "search":
		var notes []knowledge.Document
		if parts[0] == "list" {
			notes, err = store.ListNotes(activeGoal)
		} else {
			notes, err = store.SearchNotes(strings.TrimSpace(strings.TrimPrefix(command, "search")), activeGoal, 30)
		}
		if err != nil {
			return nil, err
		}
		result := &uisnapshot.Result{Kind: "list", Title: "Saved notes", Summary: fmt.Sprintf("%d notes", len(notes))}
		if len(notes) == 0 {
			result.Summary = "No matching notes."
		}
		for _, note := range notes {
			detail := note.ID
			if note.Excerpt != "" {
				detail += " · " + note.Excerpt
			}
			state := "global"
			if note.GoalID != "" {
				state = "goal"
			}
			result.Items = append(result.Items, uisnapshot.ResultItem{Label: note.Title, Detail: detail, State: state})
		}
		return result, nil
	case "show":
		if len(parts) != 2 {
			return nil, nil
		}
		note, err := store.LoadNote(parts[1])
		if err != nil {
			return nil, err
		}
		return &uisnapshot.Result{Kind: "details", Title: note.Title, Fields: []uisnapshot.Field{
			{Label: "ID", Value: note.ID}, {Label: "Goal", Value: uiEmpty(note.GoalID)},
			{Label: "Sources", Value: uiEmpty(strings.Join(note.SourceRefs, ", "))}, {Label: "Updated", Value: note.UpdatedAt.Local().Format("2006-01-02 15:04")},
		}, Markdown: note.Body}, nil
	}
	return nil, nil
}

func workSkillResult(command string) (*uisnapshot.Result, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, nil
	}
	store, _, err := knowledgeStore()
	if err != nil {
		return nil, err
	}
	switch parts[0] {
	case "list":
		all := len(parts) == 2 && parts[1] == "--all"
		skills, err := store.ListSkills(!all)
		if err != nil {
			return nil, err
		}
		result := &uisnapshot.Result{Kind: "list", Title: "Skills", Summary: fmt.Sprintf("%d skills", len(skills))}
		if len(skills) == 0 {
			result.Summary = "No skills yet."
		}
		for _, skill := range skills {
			state, tone := "enabled", "success"
			if !skill.Enabled {
				state, tone = "disabled", "muted"
			}
			result.Items = append(result.Items, uisnapshot.ResultItem{Label: skill.ID, Detail: skill.Description, State: state, Tone: tone})
		}
		return result, nil
	case "show":
		if len(parts) != 2 {
			return nil, nil
		}
		skill, err := store.LoadSkill(parts[1])
		if err != nil {
			return nil, err
		}
		state := "disabled"
		if skill.Enabled {
			state = "enabled"
		}
		return &uisnapshot.Result{Kind: "details", Title: skill.Title, Summary: skill.Description, Fields: []uisnapshot.Field{
			{Label: "State", Value: state}, {Label: "Updated", Value: skill.UpdatedAt.Local().Format("2006-01-02 15:04")},
		}, Markdown: skill.Body}, nil
	}
	return nil, nil
}

func workPlanResult(store goal.Store, activeGoal, command string) (*uisnapshot.Result, error) {
	if activeGoal == "" || strings.TrimSpace(command) != "" {
		return nil, nil
	}
	plan, err := store.LoadPlan(activeGoal)
	if err != nil {
		return nil, err
	}
	result := &uisnapshot.Result{Kind: "list", Title: "Goal plan", Fields: []uisnapshot.Field{{Label: "Goal", Value: activeGoal}, {Label: "Revision", Value: fmt.Sprint(plan.Revision)}}}
	if plan.Outcome != "" {
		result.Summary = plan.Outcome
	} else {
		result.Summary = "No plan outcome yet."
	}
	if !plan.NextReviewAt.IsZero() {
		result.Fields = append(result.Fields, uisnapshot.Field{Label: "Next review", Value: plan.NextReviewAt.Local().Format("2006-01-02 15:04")})
	}
	for _, step := range plan.Steps {
		detail := step.ID
		if !step.DueAt.IsZero() {
			detail += " · due " + step.DueAt.Local().Format("2006-01-02")
		}
		if len(step.Evidence) > 0 {
			detail += " · " + strings.Join(step.Evidence, ", ")
		}
		result.Items = append(result.Items, uisnapshot.ResultItem{Label: step.Title, Detail: detail, State: string(step.Status)})
	}
	return result, nil
}

func workActivityResult(stateDir, activeGoal string) (*uisnapshot.Result, error) {
	activity, err := workrun.ReadActivity(stateDir)
	if err != nil {
		return nil, err
	}
	result := &uisnapshot.Result{Kind: "report", Title: "Activity", Summary: fmt.Sprintf("%d running work sessions", len(activity.Runs))}
	if activeGoal != "" {
		var output bytes.Buffer
		if err := renderGoalActivity(stateDir, activeGoal, &output); err != nil {
			return nil, err
		}
		result.Sections = append(result.Sections, uisnapshot.ResultSection{Title: "Goal activity", Lines: strings.Split(strings.TrimSpace(output.String()), "\n")})
	}
	for _, run := range activity.Runs {
		section := uisnapshot.ResultSection{Title: run.Objective, Fields: []uisnapshot.Field{{Label: "Run", Value: run.ID}, {Label: "Status", Value: run.LiveStatus}}}
		for _, task := range run.Tasks {
			section.Items = append(section.Items, uisnapshot.ResultItem{Label: task.Role, Detail: task.ID + " · " + task.Summary, State: task.Status})
		}
		for _, decision := range run.PendingDecisions {
			section.Items = append(section.Items, uisnapshot.ResultItem{Label: decision.Kind, Detail: fmt.Sprintf("Decision #%d", decision.ID), State: decision.LiveStatus, Tone: "warning"})
		}
		result.Sections = append(result.Sections, section)
	}
	if len(result.Sections) == 0 {
		result.Summary = "No running work."
	}
	return result, nil
}

func workReviewResult(stateDir, path string) (*uisnapshot.Result, error) {
	bundle, err := artifact.OpenBundle(path)
	if err != nil {
		return nil, err
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		return nil, err
	}
	previews, err := artifact.PreviewBundle(bundle)
	if err != nil {
		return nil, err
	}
	result := &uisnapshot.Result{Kind: "preview", Title: "Deliverable preview", Summary: fmt.Sprintf("%d files", len(previews)), Footer: "Use /save to copy deliverables after review."}
	if len(previews) == 0 {
		result.Summary = "This turn has no deliverable files."
	}
	for _, preview := range previews {
		section := uisnapshot.ResultSection{Title: preview.Path, Summary: preview.Summary, Fields: []uisnapshot.Field{{Label: "Type", Value: preview.Kind}}}
		if preview.Content != "" {
			if preview.Kind == "markdown" {
				section.Markdown = preview.Content
			} else {
				section.Lines = strings.Split(preview.Content, "\n")
			}
		}
		if preview.Truncated {
			section.Fields = append(section.Fields, uisnapshot.Field{Label: "Preview", Value: "Truncated to a safe length", Tone: "warning"})
		}
		result.Sections = append(result.Sections, section)
	}
	for _, candidate := range bundle.Manifest.Candidates {
		result.Sections = append(result.Sections, uisnapshot.ResultSection{Title: "Proposed change " + candidate.ID, Summary: candidate.Status, Items: []uisnapshot.ResultItem{{Label: "Changed files", Detail: strings.Join(candidate.ChangedPaths, ", ")}}})
	}
	store, err := delivery.Open(stateDir); if err != nil { return nil, err }
	records, err := store.ListWork(bundle.Manifest.RunID); if err != nil { return nil, err }
	for _, record := range records {
		section := uisnapshot.ResultSection{Title: "Delivery " + record.ID, Fields: []uisnapshot.Field{{Label: "Target", Value: record.TargetPath}}}
		for _, effect := range record.Effects {
			section.Items = append(section.Items, uisnapshot.ResultItem{Label: effect.SourcePath, State: string(effect.Status)})
		}
		result.Sections = append(result.Sections, section)
	}
	return result, nil
}

func uiEmpty(value string) string {
	if value == "" {
		return "None"
	}
	return value
}
func containsWord(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func uiScope(defaultGoal string, args []string) (string, []string) {
	goalID := defaultGoal
	remaining := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		if args[index] == "--goal" && index+1 < len(args) {
			goalID = args[index+1]
			index++
			continue
		}
		if strings.HasPrefix(args[index], "--goal=") {
			goalID = strings.TrimPrefix(args[index], "--goal=")
			continue
		}
		remaining = append(remaining, args[index])
	}
	return goalID, remaining
}
