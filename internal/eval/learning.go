package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workrun"
)

const LearningDatasetVersion = 1

// LearningDataset is a small deterministic product-evaluation corpus. Its
// cases state observable future-Work behavior rather than model-quality
// assertions: which scoped context reaches a real Work request and which must
// not. Held-out cases share the established Work-evaluation split values.
type LearningDataset struct {
	Version int            `json:"version"`
	ID      string         `json:"id"`
	Target  string         `json:"target"`
	Cases   []LearningCase `json:"cases"`
}

type LearningCase struct {
	ID                string            `json:"id"`
	Family            string            `json:"family"`
	Split             string            `json:"split"`
	Project           string            `json:"project"`
	Learnings         []LearningSeed    `json:"learnings,omitempty"`
	Observations      []ObservationSeed `json:"observations,omitempty"`
	Phases            []LearningPhase   `json:"phases"`
	ExpectedLearnings int               `json:"expected_learnings"`
}

// LearningSeed represents only the inspectable learning state needed for an
// evaluation. It deliberately does not reproduce any chat transcript or Work
// payload. A project scope is the case's project; global is the only broader
// scope supported by the product today.
type LearningSeed struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Key        string `json:"key"`
	Content    string `json:"content"`
	Scope      string `json:"scope"`
	Origin     string `json:"origin"`
	Status     string `json:"status"`
	Confidence int    `json:"confidence,omitempty"`
	Confirmed  bool   `json:"confirmed,omitempty"`
	WorkID     string `json:"work_id"`
}

type ObservationSeed struct {
	ID      string `json:"id"`
	Signal  string `json:"signal"`
	WorkID  string `json:"work_id"`
	Summary string `json:"summary"`
}

type LearningPhase struct {
	ID        string           `json:"id"`
	Mutations []LearningChange `json:"mutations,omitempty"`
	Probes    []LearningProbe  `json:"probes"`
}

type LearningChange struct {
	Action     string `json:"action"`
	LearningID string `json:"learning_id"`
}

// LearningProbe is evaluated against a real deterministic Work request. A
// positive expectation is also tested against an ablated no-learning control,
// so a passing result describes a behavior change attributable to the active
// learning rather than mere record presence.
type LearningProbe struct {
	ID      string   `json:"id"`
	Project string   `json:"project"`
	Expect  []string `json:"expect,omitempty"`
	Forbid  []string `json:"forbid,omitempty"`
	Ablate  bool     `json:"ablate,omitempty"`
}

type LearningDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type LearningTrial struct {
	Version               int                  `json:"version"`
	ID                    string               `json:"id"`
	CaseID                string               `json:"case_id"`
	Family                string               `json:"family"`
	Split                 string               `json:"split"`
	Phase                 string               `json:"phase"`
	Probe                 string               `json:"probe"`
	Status                string               `json:"status"`
	ActiveLearningIDs     []string             `json:"active_learning_ids,omitempty"`
	LearningCount         int                  `json:"learning_count"`
	RetainedLearningCount int                  `json:"retained_learning_count"`
	ProvenanceWorkIDs     []string             `json:"provenance_work_ids,omitempty"`
	Ablated               bool                 `json:"ablated"`
	Diagnostics           []LearningDiagnostic `json:"diagnostics,omitempty"`
}

type LearningExperiment struct {
	Version       int             `json:"version"`
	ID            string          `json:"id"`
	Dataset       string          `json:"dataset"`
	DatasetSHA256 string          `json:"dataset_sha256"`
	Target        string          `json:"target"`
	Harness       string          `json:"harness"`
	Split         string          `json:"split"`
	Trials        []LearningTrial `json:"trials"`
	Categories    map[string]int  `json:"categories"`
	Passed        int             `json:"passed"`
	Total         int             `json:"total"`
}

type LearningEvalOptions struct {
	ID, Harness, ReportDir, Split string
}

func LoadLearningDataset(path string) (LearningDataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LearningDataset{}, err
	}
	if len(data) > 512*1024 {
		return LearningDataset{}, errors.New("learning dataset manifest too large")
	}
	var dataset LearningDataset
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dataset); err != nil {
		return LearningDataset{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return LearningDataset{}, errors.New("expected one learning dataset JSON document")
	}
	if dataset.Version != LearningDatasetVersion || dataset.Target != "learning.v1" || !identifierPattern.MatchString(dataset.ID) || len(dataset.Cases) == 0 || len(dataset.Cases) > 64 {
		return LearningDataset{}, errors.New("invalid learning dataset")
	}
	seen := map[string]bool{}
	for _, item := range dataset.Cases {
		if !identifierPattern.MatchString(item.ID) || seen[item.ID] || strings.TrimSpace(item.Family) == "" || (item.Split != WorkSplitDevelopment && item.Split != WorkSplitHeldOut) || !validLearningProject(item.Project) || len(item.Phases) == 0 || item.ExpectedLearnings < 0 {
			return LearningDataset{}, fmt.Errorf("invalid learning case %q", item.ID)
		}
		seen[item.ID] = true
		if err := validateLearningSeeds(item); err != nil {
			return LearningDataset{}, fmt.Errorf("learning case %q: %w", item.ID, err)
		}
	}
	return dataset, nil
}

func validLearningProject(value string) bool {
	return value != "" && !filepath.IsAbs(value) && filepath.Clean(value) == value && value != "." && !strings.HasPrefix(value, ".."+string(filepath.Separator)) && value != ".."
}

func validateLearningSeeds(item LearningCase) error {
	seedIDs := map[string]bool{}
	for _, seed := range item.Learnings {
		if !identifierPattern.MatchString(seed.ID) || seedIDs[seed.ID] || !validLearningType(seed.Type) || strings.TrimSpace(seed.Key) == "" || strings.TrimSpace(seed.Content) == "" || (seed.Scope != "global" && seed.Scope != "project") || (seed.Origin != string(learning.UserAuthored) && seed.Origin != string(learning.Inferred)) || !identifierPattern.MatchString(seed.WorkID) {
			return errors.New("learning seed is invalid")
		}
		if seed.Status != "" && !validLearningStatus(seed.Status) {
			return errors.New("learning seed status is invalid")
		}
		if seed.Origin == string(learning.Inferred) && seed.Status == string(learning.Active) && !seed.Confirmed {
			return errors.New("active inferred learning seed requires confirmation")
		}
		if seed.Origin == string(learning.Inferred) && (seed.Confidence < 0 || seed.Confidence > 100) {
			return errors.New("inferred learning seed confidence is invalid")
		}
		seedIDs[seed.ID] = true
	}
	observationIDs := map[string]bool{}
	for _, observation := range item.Observations {
		if !identifierPattern.MatchString(observation.ID) || observationIDs[observation.ID] || !validLearningSignal(observation.Signal) || !identifierPattern.MatchString(observation.WorkID) || strings.TrimSpace(observation.Summary) == "" {
			return errors.New("observation seed is invalid")
		}
		observationIDs[observation.ID] = true
	}
	phaseIDs := map[string]bool{}
	for _, phase := range item.Phases {
		if !identifierPattern.MatchString(phase.ID) || phaseIDs[phase.ID] || len(phase.Probes) == 0 {
			return errors.New("learning phase is invalid")
		}
		phaseIDs[phase.ID] = true
		for _, mutation := range phase.Mutations {
			if (mutation.Action != "enable" && mutation.Action != "disable" && mutation.Action != "reject") || !seedIDs[mutation.LearningID] {
				return errors.New("learning mutation is invalid")
			}
		}
		probeIDs := map[string]bool{}
		for _, probe := range phase.Probes {
			if !identifierPattern.MatchString(probe.ID) || probeIDs[probe.ID] || !validLearningProject(probe.Project) || (len(probe.Expect) == 0 && len(probe.Forbid) == 0) || (probe.Ablate && len(probe.Expect) == 0) || !validProbeTerms(probe.Expect) || !validProbeTerms(probe.Forbid) {
				return errors.New("learning probe is invalid")
			}
			probeIDs[probe.ID] = true
		}
	}
	return nil
}

func validLearningType(value string) bool {
	switch learning.Type(value) {
	case learning.Preference, learning.EnvironmentFact, learning.Procedure, learning.FailurePrevention:
		return true
	default:
		return false
	}
}

func validLearningStatus(value string) bool {
	switch learning.Status(value) {
	case learning.Candidate, learning.Active, learning.Disabled, learning.Rejected:
		return true
	default:
		return false
	}
}

func validLearningSignal(value string) bool {
	switch learning.Signal(value) {
	case learning.WorkFailed, learning.VerificationFailed, learning.DeliveryFailed, learning.DeliveryUnknown, learning.ExternalOutcomeUnknown, learning.UserAccepted, learning.UserRejected, learning.UserCorrected, learning.UserRemembered, learning.UserDeclinedLearning:
		return true
	default:
		return false
	}
}

func validProbeTerms(values []string) bool {
	if len(values) > 16 {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > 512 || strings.ContainsRune(value, 0) {
			return false
		}
	}
	return true
}

func selectLearningCases(dataset LearningDataset, requested string) (string, []LearningCase, error) {
	split := strings.TrimSpace(requested)
	if split == "" {
		split = WorkSplitDevelopment
	}
	if split != WorkSplitDevelopment && split != WorkSplitHeldOut && split != WorkSplitAll {
		return "", nil, errors.New("learning evaluation split must be development, held-out, or all")
	}
	cases := make([]LearningCase, 0, len(dataset.Cases))
	for _, item := range dataset.Cases {
		if split == WorkSplitAll || item.Split == split {
			cases = append(cases, item)
		}
	}
	if len(cases) == 0 {
		return "", nil, fmt.Errorf("learning dataset has no %s cases", split)
	}
	return split, cases, nil
}

func RunLearningExperiment(ctx context.Context, datasetPath string, dataset LearningDataset, options LearningEvalOptions) (LearningExperiment, error) {
	if !identifierPattern.MatchString(options.ID) || strings.TrimSpace(options.ReportDir) == "" {
		return LearningExperiment{}, errors.New("invalid learning experiment options")
	}
	split, cases, err := selectLearningCases(dataset, options.Split)
	if err != nil {
		return LearningExperiment{}, err
	}
	if err := os.Mkdir(options.ReportDir, 0o700); err != nil {
		return LearningExperiment{}, fmt.Errorf("create new learning experiment directory: %w", err)
	}
	selected := dataset
	selected.Cases = append([]LearningCase(nil), cases...)
	report := LearningExperiment{Version: 1, ID: options.ID, Dataset: dataset.ID, DatasetSHA256: workHash(selected), Target: dataset.Target, Harness: options.Harness, Split: split, Categories: map[string]int{}}
	if err := writeJSON(filepath.Join(options.ReportDir, "dataset.json"), selected, "learning dataset"); err != nil {
		return report, err
	}
	for _, item := range cases {
		caseTrials, err := runLearningCase(ctx, options.ReportDir, item)
		if err != nil {
			return report, err
		}
		for _, trial := range caseTrials {
			report.Trials = append(report.Trials, trial)
			report.Total++
			if trial.Status == "passed" {
				report.Passed++
			} else if len(trial.Diagnostics) == 0 {
				report.Categories["unknown"]++
			} else {
				for _, diagnostic := range trial.Diagnostics {
					report.Categories[diagnostic.Code]++
				}
			}
			if err := writeJSON(filepath.Join(options.ReportDir, trial.ID+".json"), trial, "learning trial"); err != nil {
				return report, err
			}
		}
		if err := writeJSON(filepath.Join(options.ReportDir, "experiment.json"), report, "learning experiment checkpoint"); err != nil {
			return report, err
		}
	}
	if err := os.WriteFile(filepath.Join(options.ReportDir, "report.txt"), []byte(LearningSummary(report)), 0o600); err != nil {
		return report, err
	}
	return report, writeJSON(filepath.Join(options.ReportDir, "experiment.json"), report, "learning experiment")
}

func runLearningCase(ctx context.Context, reportDir string, item LearningCase) ([]LearningTrial, error) {
	state, err := os.MkdirTemp(reportDir, "learning-state-")
	if err != nil {
		return nil, err
	}
	caseProject := filepath.Join(state, "sources", item.Project)
	if err := os.MkdirAll(caseProject, 0o700); err != nil {
		return nil, err
	}
	store, err := learning.Open(state)
	if err != nil {
		return nil, err
	}
	for _, seed := range item.Learnings {
		if _, err := createLearningSeed(store, caseProject, item, seed); err != nil {
			return nil, err
		}
	}
	for _, seed := range item.Observations {
		if _, err := store.RecordObservation(learning.ObservationInput{ID: seed.ID, Signal: learning.Signal(seed.Signal), WorkID: seed.WorkID, Scope: &learning.Scope{Kind: learning.Project, Value: caseProject}, Summary: seed.Summary, EvidenceRefs: []string{"eval/" + item.ID + "/" + seed.ID}}); err != nil {
			return nil, err
		}
	}
	records, err := store.List()
	if err != nil {
		return nil, err
	}
	if len(records) != item.ExpectedLearnings {
		return nil, fmt.Errorf("learning case %q retained %d learnings, expected %d", item.ID, len(records), item.ExpectedLearnings)
	}
	trials := make([]LearningTrial, 0)
	for _, phase := range item.Phases {
		for _, mutation := range phase.Mutations {
			if err := mutateLearning(store, mutation); err != nil {
				return nil, err
			}
		}
		for _, probe := range phase.Probes {
			trials = append(trials, runLearningProbe(ctx, state, store, item, phase, probe))
		}
	}
	return trials, nil
}

func createLearningSeed(store learning.Store, project string, item LearningCase, seed LearningSeed) (learning.Record, error) {
	scope := learning.Scope{Kind: learning.Global}
	if seed.Scope == "project" {
		scope = learning.Scope{Kind: learning.Project, Value: project}
	}
	status := learning.Status(seed.Status)
	if status == "" {
		if seed.Origin == string(learning.Inferred) {
			status = learning.Candidate
		} else {
			status = learning.Active
		}
	}
	confidence := seed.Confidence
	if seed.Origin == string(learning.Inferred) && confidence == 0 {
		confidence = 100
	}
	provenance := learning.Provenance{WorkIDs: []string{seed.WorkID}, EvidenceRefs: []string{"eval/" + item.ID + "/" + seed.ID}}
	if learning.Origin(seed.Origin) == learning.Inferred && status == learning.Active && seed.Confirmed {
		provenance.UserConfirmedAt = time.Unix(0, 0).UTC()
	}
	return store.Create(learning.Create{ID: seed.ID, Type: learning.Type(seed.Type), Key: seed.Key, Content: seed.Content, Scope: scope, Status: status, Origin: learning.Origin(seed.Origin), Confidence: confidence, Provenance: provenance})
}

func mutateLearning(store learning.Store, mutation LearningChange) error {
	switch mutation.Action {
	case "enable":
		_, err := store.Enable(mutation.LearningID)
		return err
	case "disable":
		_, err := store.Disable(mutation.LearningID)
		return err
	case "reject":
		_, err := store.Reject(mutation.LearningID)
		return err
	default:
		return errors.New("unknown learning mutation")
	}
}

func runLearningProbe(ctx context.Context, state string, store learning.Store, item LearningCase, phase LearningPhase, probe LearningProbe) LearningTrial {
	trial := LearningTrial{Version: 1, ID: "learning-" + item.ID + "-" + phase.ID + "-" + probe.ID, CaseID: item.ID, Family: item.Family, Split: item.Split, Phase: phase.ID, Probe: probe.ID, Status: "failed", Ablated: probe.Ablate}
	project := filepath.Join(state, "sources", probe.Project)
	if err := os.MkdirAll(project, 0o700); err != nil {
		trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "setup_error", Message: err.Error()})
		return trial
	}
	prompt, records, err := futureWorkPrompt(ctx, state, store, trial.ID, project)
	trial.LearningCount = len(records)
	for _, record := range records {
		trial.ActiveLearningIDs = append(trial.ActiveLearningIDs, record.ID)
	}
	sort.Strings(trial.ActiveLearningIDs)
	if err != nil {
		trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "work_probe_error", Message: err.Error()})
		return trial
	}
	retained, listErr := store.List()
	if listErr != nil {
		trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "provenance_read_error", Message: listErr.Error()})
		return trial
	}
	trial.RetainedLearningCount = len(retained)
	for _, record := range retained {
		if len(record.Provenance.WorkIDs) == 0 {
			trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "provenance_missing", Message: fmt.Sprintf("learning %s has no Work provenance", record.ID)})
			continue
		}
		trial.ProvenanceWorkIDs = append(trial.ProvenanceWorkIDs, record.Provenance.WorkIDs...)
	}
	sort.Strings(trial.ProvenanceWorkIDs)
	for _, expected := range probe.Expect {
		if !strings.Contains(prompt, expected) {
			trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "retention_missing", Message: fmt.Sprintf("expected active learning content %q was absent from future Work", expected)})
		}
	}
	for _, forbidden := range probe.Forbid {
		if strings.Contains(prompt, forbidden) {
			trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "scope_leak", Message: fmt.Sprintf("learning content %q reached unrelated or inactive future Work", forbidden)})
		}
	}
	if probe.Ablate {
		controlState, err := os.MkdirTemp(filepath.Dir(state), "learning-control-")
		if err != nil {
			trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "control_setup_error", Message: err.Error()})
		} else {
			control, openErr := learning.Open(controlState)
			if openErr != nil {
				trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "control_setup_error", Message: openErr.Error()})
			} else {
				controlProject := filepath.Join(controlState, "sources", probe.Project)
				if mkdirErr := os.MkdirAll(controlProject, 0o700); mkdirErr != nil {
					trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "control_setup_error", Message: mkdirErr.Error()})
				} else if controlPrompt, _, controlErr := futureWorkPrompt(ctx, controlState, control, trial.ID+"-control", controlProject); controlErr != nil {
					trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "control_work_error", Message: controlErr.Error()})
				} else {
					for _, expected := range probe.Expect {
						if strings.Contains(controlPrompt, expected) {
							trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "ablation_no_effect", Message: fmt.Sprintf("control without learning still contained %q", expected)})
						}
					}
				}
			}
		}
	}
	if len(trial.Diagnostics) == 0 {
		trial.Status = "passed"
	}
	return trial
}

func futureWorkPrompt(ctx context.Context, state string, store learning.Store, runID, project string) (string, []learning.Record, error) {
	records, err := store.Projection(learning.Context{Project: project})
	if err != nil {
		return "", nil, err
	}
	model := &learningPromptModel{}
	_, err = (workrun.Executor{Model: model, StateDir: state}).Execute(ctx, workrun.Request{
		RunID: runID, SourcePath: project, Objective: "Inspect the selected project without changing it.",
		Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 1,
		LearningContext: learning.RenderProjection(records),
	})
	return model.system, records, err
}

type learningPromptModel struct{ system string }

func (m *learningPromptModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.system = request.System
	return agent.Turn{Text: "Inspection complete."}, nil
}

func LearningSummary(report LearningExperiment) string {
	return fmt.Sprintf("%s: %d/%d deterministic learning probes passed. Split: %s. Diagnostics: %v.\n", report.ID, report.Passed, report.Total, report.Split, report.Categories)
}

func LoadLearningExperiment(path string) (LearningExperiment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LearningExperiment{}, err
	}
	var report LearningExperiment
	if err := json.Unmarshal(data, &report); err != nil {
		return LearningExperiment{}, err
	}
	if report.Version != 1 || !identifierPattern.MatchString(report.ID) || report.Target != "learning.v1" {
		return LearningExperiment{}, errors.New("invalid learning experiment")
	}
	return report, nil
}
