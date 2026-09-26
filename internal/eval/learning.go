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

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
)

const LearningDatasetVersion = 1

// LearningDataset is a small deterministic product-evaluation corpus. Its
// cases state observable future-Work behavior rather than model-quality
// assertions. Every probe runs the Work agent loop with a deterministic model
// that receives learning only through the normal system context.
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
	ID        string   `json:"id"`
	Project   string   `json:"project"`
	Objective string   `json:"objective"`
	Expect    []string `json:"expect,omitempty"`
	Forbid    []string `json:"forbid,omitempty"`
	Ablate    bool     `json:"ablate,omitempty"`
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
	WorkID                string               `json:"work_id,omitempty"`
	ArtifactPath          string               `json:"artifact_path,omitempty"`
	Output                string               `json:"output,omitempty"`
	ControlOutput         string               `json:"control_output,omitempty"`
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
			if !identifierPattern.MatchString(probe.ID) || probeIDs[probe.ID] || !validLearningProject(probe.Project) || !validLearningObjective(probe.Objective) || (len(probe.Expect) == 0 && len(probe.Forbid) == 0) || (probe.Ablate && len(probe.Expect) == 0) || !validProbeTerms(probe.Expect) || !validProbeTerms(probe.Forbid) {
				return errors.New("learning probe is invalid")
			}
			probeIDs[probe.ID] = true
		}
	}
	return nil
}

func validLearningObjective(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 4096 && !strings.ContainsRune(value, 0)
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
	result, records, err := futureWorkBehavior(ctx, state, store, trial.ID, project, probe.Objective)
	trial.LearningCount = len(records)
	for _, record := range records {
		trial.ActiveLearningIDs = append(trial.ActiveLearningIDs, record.ID)
	}
	sort.Strings(trial.ActiveLearningIDs)
	if err != nil {
		trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "work_probe_error", Message: err.Error()})
		return trial
	}
	trial.WorkID = result.WorkID
	trial.ArtifactPath = result.ArtifactPath
	trial.Output = result.Output
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
		if !strings.Contains(trial.Output, expected) {
			trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "relevant_transfer", Message: fmt.Sprintf("expected behavior %q; observed %q", expected, trial.Output)})
		}
	}
	for _, forbidden := range probe.Forbid {
		if strings.Contains(trial.Output, forbidden) {
			trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "scope_leak", Message: fmt.Sprintf("forbidden behavior %q; observed %q", forbidden, trial.Output)})
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
				} else if controlResult, _, controlErr := futureWorkBehavior(ctx, controlState, control, trial.ID+"-control", controlProject, probe.Objective); controlErr != nil {
					trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "control_work_error", Message: controlErr.Error()})
				} else {
					trial.ControlOutput = controlResult.Output
					for _, expected := range probe.Expect {
						if strings.Contains(controlResult.Output, expected) {
							trial.Diagnostics = append(trial.Diagnostics, LearningDiagnostic{Code: "ablation_no_effect", Message: fmt.Sprintf("control without learning still produced %q: %q", expected, controlResult.Output)})
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

type learningBehaviorResult struct {
	WorkID       string
	ArtifactPath string
	Output       string
}

func futureWorkBehavior(ctx context.Context, state string, store learning.Store, runID, project, objective string) (learningBehaviorResult, []learning.Record, error) {
	records, err := store.Projection(learning.Context{Project: project})
	if err != nil {
		return learningBehaviorResult{}, nil, err
	}
	model := &learningBehaviorModel{}
	outcome, err := (workrun.Service{Executor: workrun.Executor{Model: model, StateDir: state}}).Execute(ctx, workrun.Request{
		RunID: runID, SourcePath: project, Objective: objective,
		Contract: artifact.DefaultContract("decision.txt"), MaxSteps: 3,
		LearningContext: learning.RenderProjection(records),
	})
	if err != nil {
		return learningBehaviorResult{}, records, err
	}
	output, err := outcome.Work.Output.ReadRegularFile("decision.txt", 64*1024)
	if err != nil {
		return learningBehaviorResult{}, records, err
	}
	history, err := workhistory.Open(state)
	if err != nil {
		return learningBehaviorResult{}, records, err
	}
	record, err := history.Load(outcome.RevisionID)
	if err != nil {
		return learningBehaviorResult{}, records, fmt.Errorf("load Work history: %w", err)
	}
	if record.RevisionID != outcome.RevisionID || record.Evidence.ArtifactManifestPath != outcome.Work.ManifestPath {
		return learningBehaviorResult{}, records, errors.New("Work history does not reference the evaluated artifact")
	}
	return learningBehaviorResult{WorkID: outcome.RevisionID, ArtifactPath: "decision.txt", Output: string(output)}, records, nil
}

// learningBehaviorModel is a deterministic stand-in for a model. It reads
// only the system and user messages Work supplies; it does not receive a
// learning store, learning records, or test-case identity. Its small policy is
// intentionally generic so the evaluator can grade a sealed Work artifact.
type learningBehaviorModel struct{ decision string }

func (m *learningBehaviorModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	if m.decision == "" {
		m.decision = learningBehaviorDecision(request.System, latestUserObjective(request.Messages))
		arguments, err := json.Marshal(map[string]string{"path": "decision.txt", "content": m.decision})
		if err != nil {
			return agent.Turn{}, err
		}
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "learning-decision", Name: "write_artifact", Arguments: arguments}}}, nil
	}
	return agent.Turn{Text: "Recorded " + m.decision + "."}, nil
}

func latestUserObjective(messages []agent.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == agent.RoleUser {
			return messages[index].Content
		}
	}
	return ""
}

func learningBehaviorDecision(system, objective string) string {
	active := activeLearningContext(system)
	return strings.Join([]string{
		"package_manager=" + behaviorPackageManager(objective, active),
		"report_format=" + behaviorReportFormat(objective, active),
		"verification_plan=" + behaviorVerificationPlan(objective, active),
		"delivery_retry=inspect-before-retry",
	}, "; ")
}

func activeLearningContext(system string) string {
	const marker = "Applicable active learnings:\n"
	start := strings.Index(system, marker)
	if start < 0 {
		return ""
	}
	context := system[start+len(marker):]
	if end := strings.Index(context, "\n\n"); end >= 0 {
		context = context[:end]
	}
	return strings.ToLower(context)
}

func behaviorPackageManager(objective, active string) string {
	if selected := packageManagerFrom(strings.ToLower(objective)); selected != "" {
		return selected
	}
	if selected := packageManagerFrom(active); selected != "" {
		return selected
	}
	return "npm"
}

func packageManagerFrom(value string) string {
	for _, manager := range []string{"pnpm", "yarn", "bun", "npm"} {
		if strings.Contains(value, manager) {
			return manager
		}
	}
	return ""
}

func behaviorReportFormat(objective, active string) string {
	if selected := reportFormatFrom(strings.ToLower(objective)); selected != "" {
		return selected
	}
	if selected := reportFormatFrom(active); selected != "" {
		return selected
	}
	return "pdf"
}

func reportFormatFrom(value string) string {
	for _, format := range []string{"csv", "markdown", "html", "pdf"} {
		if strings.Contains(value, format) {
			return format
		}
	}
	return ""
}

func behaviorVerificationPlan(objective, active string) string {
	value := strings.ToLower(objective + "\n" + active)
	if strings.Contains(value, "api package tests") {
		return "api-package-tests"
	}
	if strings.Contains(value, "web package tests") {
		return "web-package-tests"
	}
	return "standard"
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
