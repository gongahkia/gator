package eval

import (
	"context"
	"errors"
	"github.com/gongahkia/gator/internal/telemetry"
	"net/http"
	"strings"
	"time"
)

// ExportLangSmith associates redacted dataset examples, experiment runs and scores.
// It never uploads objectives, source contents, grader answers or private transcripts.
func ExportLangSmith(ctx context.Context, client *http.Client, endpoint, key string, report WorkExperiment) error {
	if key == "" || endpoint == "" {
		return errors.New("LangSmith endpoint and API key are required")
	}
	endpoint = strings.TrimSuffix(endpoint, "/")
	post := func(path string, value, result any) error {
		return telemetry.Post(ctx, client, endpoint+path, key, value, result)
	}
	var dataset struct {
		ID string `json:"id"`
	}
	if err := post("/datasets", map[string]any{"name": report.Dataset + "-" + report.ID, "description": "Gator redacted evaluation metadata", "data_type": "kv"}, &dataset); err != nil {
		return err
	}
	if dataset.ID == "" {
		return errors.New("LangSmith returned no dataset ID")
	}
	examples := map[string]string{}
	for _, trial := range report.Trials {
		if examples[trial.CaseID] != "" {
			continue
		}
		var example struct {
			ID string `json:"id"`
		}
		if err := post("/examples", map[string]any{"dataset_id": dataset.ID, "inputs": map[string]string{"case_id": trial.CaseID, "case_sha256": trial.CaseSHA256}, "metadata": map[string]string{"split": trial.Split, "family": trial.Family}}, &example); err != nil {
			return err
		}
		if example.ID == "" {
			return errors.New("LangSmith returned no example ID")
		}
		examples[trial.CaseID] = example.ID
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := post("/sessions", map[string]any{"name": report.ID, "reference_dataset_id": dataset.ID, "start_time": time.Now().UTC(), "extra": map[string]any{"metadata": map[string]any{"harness": report.Harness, "scripted": report.Scripted, "delegation": report.Delegation}}}, &session); err != nil {
		return err
	}
	if session.ID == "" {
		return errors.New("LangSmith returned no experiment ID")
	}
	for _, trial := range report.Trials {
		id := telemetry.ID(report.ID+"/"+trial.ID, 32)
		run := map[string]any{"id": id, "name": trial.CaseID, "run_type": "chain", "start_time": time.Now().UTC(), "end_time": time.Now().UTC(), "reference_example_id": examples[trial.CaseID], "session_id": session.ID, "inputs": map[string]string{"case_sha256": trial.CaseSHA256}, "outputs": map[string]any{"status": trial.Status, "category": trial.Category, "model_requests": trial.Usage.ModelRequests}}
		if err := post("/runs", run, nil); err != nil {
			return err
		}
		spanIDs := map[string]string{}
		for _, span := range trial.Spans {
			spanIDs[span.ID] = telemetry.ID(id+"/"+span.ID, 32)
		}
		for _, span := range trial.Spans {
			parent := id
			if span.Parent != "" {
				parent = spanIDs[span.Parent]
				if parent == "" {
					return errors.New("trace has unknown parent span")
				}
			}
			end := span.End
			if end.IsZero() {
				end = span.Start
			}
			kind := "chain"
			if span.Name == "model_attempt" {
				kind = "llm"
			} else if strings.HasPrefix(span.Name, "tool/") {
				kind = "tool"
			}
			mapped := map[string]any{"id": spanIDs[span.ID], "parent_run_id": parent, "name": span.Name, "run_type": kind, "start_time": span.Start, "end_time": end, "session_id": session.ID, "reference_example_id": examples[trial.CaseID], "inputs": map[string]any{}, "outputs": map[string]any{"step": span.Step, "attempt": span.Attempt, "usage": span.Usage}}
			if err := post("/runs", mapped, nil); err != nil {
				return err
			}
		}
		child := map[string]any{"id": telemetry.ID(id+"/grading", 32), "parent_run_id": id, "name": "independent grading", "run_type": "chain", "start_time": time.Now().UTC(), "end_time": time.Now().UTC(), "session_id": session.ID, "reference_example_id": examples[trial.CaseID], "inputs": map[string]any{}, "outputs": map[string]any{"grader_count": len(trial.Grades)}}
		if err := post("/runs", child, nil); err != nil {
			return err
		}
		for _, grade := range trial.Grades {
			score := 0
			if grade.Passed {
				score = 1
			}
			if err := post("/feedback", map[string]any{"run_id": id, "session_id": session.ID, "key": grade.Kind, "score": score, "source_info": map[string]any{"grader_version": 1}}, nil); err != nil {
				return err
			}
		}
	}
	return nil
}
