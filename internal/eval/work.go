package eval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/xuri/excelize/v2"
)

const WorkDatasetVersion = 1

type WorkDataset struct {
	Version int        `json:"version"`
	ID      string     `json:"id"`
	Target  string     `json:"target"`
	Cases   []WorkCase `json:"cases"`
}
type WorkCase struct {
	ID           string                  `json:"id"`
	Family       string                  `json:"family"`
	Split        string                  `json:"split"`
	Source       string                  `json:"source"`
	Objective    string                  `json:"objective"`
	Contract     artifact.Contract       `json:"contract"`
	Mode         action.Mode             `json:"mode"`
	MaxSteps     int                     `json:"max_steps"`
	Limits       agent.Limits            `json:"limits"`
	Code         workrun.CodePolicy      `json:"code"`
	Capabilities []string                `json:"capabilities"`
	Turns        []agent.Turn            `json:"script"`
	CodeTurns    []agent.Turn            `json:"code_script,omitempty"`
	RoleTurns    map[string][]agent.Turn `json:"role_scripts,omitempty"`
	Followups    []WorkFollowup          `json:"followups,omitempty"`
	Graders      []WorkGrader            `json:"graders"`
}
type WorkFollowup struct {
	Objective string       `json:"objective"`
	Parent    int          `json:"parent"`
	Refresh   bool         `json:"refresh"`
	Turns     []agent.Turn `json:"script"`
}
type WorkGrader struct {
	Version  int    `json:"version"`
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	Sheet    string `json:"sheet,omitempty"`
	Cell     string `json:"cell,omitempty"`
	Expected string `json:"expected"`
}
type Grade struct {
	Kind     string `json:"kind"`
	Passed   bool   `json:"passed"`
	Evidence string `json:"evidence"`
	Error    string `json:"error,omitempty"`
}
type WorkTrial struct {
	Version        int         `json:"version"`
	ID             string      `json:"id"`
	CaseID         string      `json:"case_id"`
	CaseSHA256     string      `json:"case_sha256"`
	Split          string      `json:"split"`
	Family         string      `json:"family"`
	Trial          int         `json:"trial"`
	Status         string      `json:"status"`
	Category       string      `json:"category"`
	Error          string      `json:"error,omitempty"`
	Grades         []Grade     `json:"grades"`
	ConversationID string      `json:"conversation_id"`
	RevisionID     string      `json:"revision_id"`
	SnapshotID     string      `json:"snapshot_id"`
	ContractSHA256 string      `json:"contract_sha256"`
	PolicySHA256   string      `json:"policy_sha256"`
	SourceSHA256   string      `json:"source_sha256"`
	BundlePath     string      `json:"bundle_path"`
	Usage          agent.Usage `json:"usage"`
	DurationMS     int64       `json:"duration_ms"`
}
type WorkExperiment struct {
	Version         int            `json:"version"`
	ID              string         `json:"id"`
	Dataset         string         `json:"dataset"`
	DatasetSHA256   string         `json:"dataset_sha256"`
	Target          string         `json:"target"`
	Harness         string         `json:"harness"`
	Provider        string         `json:"provider"`
	Model           string         `json:"model"`
	Scripted        bool           `json:"scripted"`
	Delegation      bool           `json:"delegation"`
	Attempts        int            `json:"attempts"`
	Trials          []WorkTrial    `json:"trials"`
	Categories      map[string]int `json:"categories"`
	Passed          int            `json:"passed"`
	Total           int            `json:"total"`
	CasesAtLeastOne int            `json:"cases_at_least_one"`
	CasesAll        int            `json:"cases_all"`
}
type WorkEvalOptions struct {
	MaxRequests                             int
	ID, Harness, Provider, Model, ReportDir string
	Attempts                                int
	Delegation, Live                        bool
}
type WorkServiceFactory func(WorkCase, string) (workrun.Service, error)

func LoadWorkDataset(path string) (WorkDataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkDataset{}, err
	}
	if len(data) > 8*1024*1024 {
		return WorkDataset{}, errors.New("dataset manifest too large")
	}
	var dataset WorkDataset
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dataset); err != nil {
		return dataset, err
	}
	if dataset.Version != WorkDatasetVersion || dataset.Target != "work.v1" || !identifierPattern.MatchString(dataset.ID) || len(dataset.Cases) == 0 || len(dataset.Cases) > 128 {
		return dataset, errors.New("invalid Work dataset")
	}
	seen := map[string]bool{}
	for _, c := range dataset.Cases {
		if !identifierPattern.MatchString(c.ID) || seen[c.ID] || (c.Split != "development" && c.Split != "held-out") || c.Objective == "" || c.MaxSteps < 1 || c.MaxSteps > 128 || len(c.Graders) == 0 {
			return dataset, fmt.Errorf("invalid case %q", c.ID)
		}
		seen[c.ID] = true
		if c.Source == "" || filepath.IsAbs(c.Source) || filepath.Clean(c.Source) != c.Source || strings.HasPrefix(c.Source, "..") {
			return dataset, errors.New("fixture source must stay beneath dataset directory")
		}
		sourcePath, err := filepath.Abs(filepath.Join(filepath.Dir(path), c.Source))
		if err != nil {
			return dataset, err
		}
		root, err := filepath.EvalSymlinks(sourcePath)
		if err != nil {
			return dataset, err
		}
		base, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			return dataset, err
		}
		if !strings.HasPrefix(root, base+string(filepath.Separator)) {
			return dataset, errors.New("fixture source escapes dataset")
		}
		if err := c.Contract.Validate(); err != nil {
			return dataset, fmt.Errorf("case %s contract: %w", c.ID, err)
		}
		for _, g := range c.Graders {
			if g.Version != 1 {
				return dataset, errors.New("unsupported grader version")
			}
			switch g.Kind {
			case "file_equals", "file_contains", "cell_equals", "context_contains", "context_absent", "status", "tool_error_contains", "candidate_status", "source_unchanged", "contract_digest":
			default:
				return dataset, fmt.Errorf("unknown grader %q", g.Kind)
			}
		}
		for i, f := range c.Followups {
			if f.Parent < 0 || f.Parent > i {
				return dataset, errors.New("followup parent is not a preceding revision")
			}
		}
	}
	return dataset, nil
}
func workHash(value any) string {
	data, _ := json.Marshal(value)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func RunWorkExperiment(ctx context.Context, datasetPath string, dataset WorkDataset, options WorkEvalOptions, factory WorkServiceFactory) (WorkExperiment, error) {
	if options.Attempts < 1 || options.Attempts > 10 || !identifierPattern.MatchString(options.ID) || options.ReportDir == "" || factory == nil {
		return WorkExperiment{}, errors.New("invalid Work experiment options")
	}
	if err := os.Mkdir(options.ReportDir, 0700); err != nil {
		return WorkExperiment{}, fmt.Errorf("create new experiment directory: %w", err)
	}
	report := WorkExperiment{Version: 1, ID: options.ID, Dataset: dataset.ID, DatasetSHA256: workHash(dataset), Target: dataset.Target, Harness: options.Harness, Provider: options.Provider, Model: options.Model, Scripted: !options.Live, Delegation: options.Delegation, Attempts: options.Attempts, Categories: map[string]int{}}
	var budget *agent.Budget
	if options.MaxRequests > 0 {
		budget = &agent.Budget{Limits: agent.Limits{ModelRequests: options.MaxRequests}}
	}
	for _, c := range dataset.Cases {
		passes := 0
		for trial := 1; trial <= options.Attempts; trial++ {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			item := WorkTrial{Version: 1, ID: fmt.Sprintf("%s-%s-%02d", options.ID, c.ID, trial), CaseID: c.ID, CaseSHA256: workHash(c), Split: c.Split, Family: c.Family, Trial: trial, Status: "failed", Category: "task"}
			started := time.Now()
			state, err := os.MkdirTemp(options.ReportDir, "state-")
			if err != nil {
				return report, err
			}
			evaluated := c
			evaluated.ID = item.ID
			service, setupErr := factory(evaluated, state)
			source := filepath.Join(filepath.Dir(datasetPath), c.Source)
			request := workrun.Request{RunID: item.ID, Budget: budget, SourcePath: source, Objective: c.Objective, MaxSteps: c.MaxSteps, Contract: c.Contract, Mode: c.Mode, Code: c.Code, Limits: c.Limits, DisableDelegation: !options.Delegation}
			var outcome workrun.Outcome
			var runErr error
			if setupErr != nil {
				item.Category = "setup"
				runErr = setupErr
			} else {
				outcome, runErr = service.Execute(ctx, request)
				revisions := []string{outcome.RevisionID}
				for followIndex, follow := range c.Followups {
					request.RunID = fmt.Sprintf("%s-rev%d", item.ID, followIndex+1)
					if outcome.ConversationID == "" {
						break
					}
					request.ConversationID = outcome.ConversationID
					request.ParentRevisionID = revisions[follow.Parent]
					request.RefreshSource = follow.Refresh
					request.Objective = follow.Objective
					if !options.Live {
						service.Executor.Model = &ScriptedModel{Turns: append([]agent.Turn(nil), follow.Turns...)}
					}
					outcome, runErr = service.Execute(ctx, request)
					revisions = append(revisions, outcome.RevisionID)
				}
				if runErr != nil {
					switch {
					case errors.Is(runErr, agent.ErrBudget):
						item.Category = "budget"
					case errors.Is(runErr, context.DeadlineExceeded):
						item.Category = "timeout"
					case agent.IsTransient(runErr):
						item.Category = "provider"
					case errors.Is(runErr, context.Canceled):
						item.Category = "cancelled"
					}
				}
				all := true
				for _, grader := range c.Graders {
					grade := gradeWork(grader, outcome, runErr, c.Contract)
					item.Grades = append(item.Grades, grade)
					all = all && grade.Passed
					if grade.Error != "" {
						item.Category = "grader"
					}
				}
				if all {
					item.Status = "passed"
					item.Category = "success"
					passes++
					report.Passed++
				}
			}
			if runErr != nil {
				item.Error = runErr.Error()
			}
			item.ConversationID = outcome.ConversationID
			item.RevisionID = outcome.RevisionID
			item.SnapshotID = outcome.SnapshotID
			item.ContractSHA256 = outcome.Manifest.ContractSHA256
			item.PolicySHA256 = outcome.Manifest.PolicySHA256
			item.SourceSHA256 = outcome.SourceSnapshot.SHA256
			item.BundlePath = outcome.Work.Path
			item.Usage = outcome.Manifest.Usage
			item.DurationMS = time.Since(started).Milliseconds()
			report.Trials = append(report.Trials, item)
			report.Categories[item.Category]++
			report.Total++
			if err := writeJSON(filepath.Join(options.ReportDir, item.ID+".json"), item, "Work trial"); err != nil {
				return report, err
			}
		}
		if passes > 0 {
			report.CasesAtLeastOne++
		}
		if passes == options.Attempts {
			report.CasesAll++
		}
	}
	return report, writeJSON(filepath.Join(options.ReportDir, "experiment.json"), report, "Work experiment")
}
func gradeWork(g WorkGrader, outcome workrun.Outcome, runErr error, contract artifact.Contract) Grade {
	grade := Grade{Kind: g.Kind}
	actual := ""
	var err error
	switch g.Kind {
	case "status":
		actual = string(outcome.Manifest.Status)
		if actual == "" && runErr != nil {
			actual = "error"
		}
	case "contract_digest":
		actual = outcome.Manifest.ContractSHA256
		g.Expected, err = contract.Digest()
	case "source_unchanged":
		data, readErr := outcome.Work.Source.ReadRegularFile(g.Path, 512*1024)
		actual = string(data)
		err = readErr
	case "file_equals", "file_contains":
		data, readErr := outcome.Work.Output.ReadRegularFile(g.Path, 512*1024)
		actual = string(data)
		err = readErr
	case "cell_equals":
		data, readErr := outcome.Work.Output.ReadRegularFile(g.Path, 16*1024*1024)
		err = readErr
		if err == nil {
			file, openErr := excelize.OpenReader(bytes.NewReader(data))
			err = openErr
			if err == nil {
				actual, err = file.GetCellValue(g.Sheet, g.Cell)
				file.Close()
			}
		}
	case "context_contains", "context_absent":
		for _, message := range outcome.Result.Messages {
			actual += message.Content + "\n"
		}
	case "tool_error_contains":
		for _, event := range outcome.Events {
			actual += event.ToolError + "\n"
		}
	case "candidate_status":
		for _, candidate := range outcome.Manifest.Candidates {
			actual += candidate.Status + "\n"
		}
	}
	if err != nil {
		grade.Error = err.Error()
		return grade
	}
	switch g.Kind {
	case "file_contains", "context_contains", "tool_error_contains", "candidate_status":
		grade.Passed = strings.Contains(actual, g.Expected)
	case "context_absent":
		grade.Passed = !strings.Contains(actual, g.Expected)
	default:
		grade.Passed = actual == g.Expected
	}
	grade.Evidence = fmt.Sprintf("expected %q; observed SHA-256 %s", g.Expected, workHash(actual))
	return grade
}
func LoadWorkExperiment(path string) (WorkExperiment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkExperiment{}, err
	}
	var report WorkExperiment
	err = json.Unmarshal(data, &report)
	if err == nil && report.Version != 1 {
		err = errors.New("unsupported experiment version")
	}
	return report, err
}
func CompareWork(a, b WorkExperiment) (string, error) {
	if a.DatasetSHA256 != b.DatasetSHA256 || a.Attempts != b.Attempts {
		return "", errors.New("comparison requires identical dataset and trial counts")
	}
	scores := map[string][2]int{}
	for _, trial := range a.Trials {
		value := scores[trial.CaseID]
		if trial.Status == "passed" {
			value[0]++
		}
		scores[trial.CaseID] = value
	}
	for _, trial := range b.Trials {
		value := scores[trial.CaseID]
		if trial.Status == "passed" {
			value[1]++
		}
		scores[trial.CaseID] = value
	}
	keys := []string{}
	for key := range scores {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output strings.Builder
	fmt.Fprintf(&output, "%s: %d/%d; %s: %d/%d\n", a.ID, a.Passed, a.Total, b.ID, b.Passed, b.Total)
	for _, key := range keys {
		score := scores[key]
		fmt.Fprintf(&output, "%s: %d -> %d of %d trials\n", key, score[0], score[1], a.Attempts)
	}
	fmt.Fprintf(&output, "At least one success: %d -> %d cases; all trials succeed: %d -> %d cases.\n", a.CasesAtLeastOne, b.CasesAtLeastOne, a.CasesAll, b.CasesAll)
	return output.String(), nil
}
