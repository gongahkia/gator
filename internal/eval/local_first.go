package eval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/workrun"
)

// LocalFirstProfile selects reference transcripts, not live model adapters.
// Every arm uses RunWorkExperiment and the existing Work/split/budget contracts.
type LocalFirstProfile struct {
	digest        string
	Version       int                      `json:"version"`
	ID            string                   `json:"id"`
	ContextTokens int                      `json:"context_tokens"`
	Concurrency   int                      `json:"concurrency"`
	MaxRequests   int                      `json:"max_requests"`
	WallSeconds   int                      `json:"wall_seconds"`
	Arms          map[string]LocalFirstArm `json:"arms"`
}

type LocalFirstArm struct {
	Dataset string `json:"dataset"`
	SHA256  string `json:"sha256"`
}

func LoadLocalFirstProfile(path string) (LocalFirstProfile, error) {
	var p LocalFirstProfile
	data, err := readLocalFirstFile(path, 64*1024)
	if err != nil {
		return p, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&p); err != nil {
		return p, errors.New("invalid local-first profile fields")
	}
	if d.Decode(new(any)) != io.EOF || p.Version != 1 || !identifierPattern.MatchString(p.ID) || p.ContextTokens < 1024 || p.ContextTokens > 8192 || p.Concurrency != 1 || p.MaxRequests < 1 || p.MaxRequests > 512 || p.WallSeconds < 1 || p.WallSeconds > 120 || len(p.Arms) != 4 {
		return p, errors.New("invalid local-first profile version or bounds")
	}
	for _, arm := range []string{"A", "B", "C", "D"} {
		v, ok := p.Arms[arm]
		if !ok || len(v.SHA256) != 64 {
			return p, errors.New("local-first requires four hashed arms")
		}
		if _, err := hex.DecodeString(v.SHA256); err != nil {
			return p, errors.New("invalid local-first dataset digest")
		}
	}
	sum := sha256.Sum256(data)
	p.digest = hex.EncodeToString(sum[:])
	return p, nil
}

func readLocalFirstFile(path string, maximum int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("evaluation input must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if err == nil && int64(len(data)) > maximum {
		err = errors.New("evaluation input exceeds size bound")
	}
	return data, err
}

// LoadLocalFirstArm rejects network/code/CLI fixture scripts before any runner
// is constructed. Deliberately malformed read arguments exercise Go rejection.
func LoadLocalFirstArm(profilePath string, p LocalFirstProfile, arm string) (string, WorkDataset, string, error) {
	a, ok := p.Arms[arm]
	if !ok {
		return "", WorkDataset{}, "", errors.New("architecture must be A, B, C or D")
	}
	if filepath.IsAbs(a.Dataset) || filepath.Clean(a.Dataset) != a.Dataset || a.Dataset == "." || strings.HasPrefix(a.Dataset, "..") {
		return "", WorkDataset{}, "", errors.New("dataset must stay beneath profile")
	}
	base, err := filepath.EvalSymlinks(filepath.Dir(profilePath))
	if err != nil {
		return "", WorkDataset{}, "", err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return "", WorkDataset{}, "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(base, a.Dataset))
	if err != nil || !strings.HasPrefix(path, base+string(filepath.Separator)) {
		return "", WorkDataset{}, "", errors.New("dataset escapes profile directory")
	}
	data, err := readLocalFirstFile(path, 8*1024*1024)
	if err != nil {
		return "", WorkDataset{}, "", err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != a.SHA256 {
		return "", WorkDataset{}, "", errors.New("local-first dataset digest changed")
	}
	if _, err := decodeJSONValue(data); err != nil {
		return "", WorkDataset{}, "", errors.New("local-first dataset must be one JSON document")
	}
	dataset, err := loadWorkDatasetData(path, data)
	if err != nil {
		return "", dataset, "", err
	}
	type commonCase struct {
		ID, Family, Split, Source, Fixture, Objective, Contract, Limits, Mode string
		Steps                                                                 int
		Followups                                                             []string
	}
	common := []commonCase{}
	for _, c := range dataset.Cases {
		if !oneOf(string(c.Mode), "draft", "inspect") || c.EnableCode || len(c.WebOrigins) != 0 || len(c.Capabilities) != 0 || len(c.RoleTurns) != 0 || len(c.CodeTurns) != 0 || c.Goal != nil || !reflect.DeepEqual(c.Code, workrun.CodePolicy{}) || c.Contract.ExternalActions != "forbid" || c.MaxSteps > 8 || c.Limits.ModelRequests < 1 || c.Limits.ModelRequests > 8 || c.Limits.WallSeconds < 1 || c.Limits.WallSeconds > 10 {
			return "", dataset, "", errors.New("local-first fixtures forbid execution/network authority and require small budgets")
		}
		if err := localFirstTurns(c.Turns); err != nil {
			return "", dataset, "", err
		}
		if len(c.Followups) > 2 {
			return "", dataset, "", errors.New("local-first followup bound exceeded")
		}
		for _, grader := range c.Graders {
			if !oneOf(grader.Kind, "local_first_decision", "tool_call_count", "tool_error_contains", "source_unchanged", "final_contains", "final_excludes", "status") {
				return "", dataset, "", errors.New("unsupported local-first fixture grader")
			}
		}
		var followups []string
		for _, f := range c.Followups {
			if f.Refresh || len(f.SourceEdits) != 0 || len(f.PlanChanges) != 0 || f.GoalContext != "" {
				return "", dataset, "", errors.New("local-first followups cannot mutate inputs or plans")
			}
			if err := localFirstTurns(f.Turns); err != nil {
				return "", dataset, "", err
			}
			followups = append(followups, f.Objective)
		}
		common = append(common, commonCase{c.ID, c.Family, c.Split, c.Source, c.FixtureSHA256, c.Objective, workHash(c.Contract), workHash(c.Limits), string(c.Mode), c.MaxSteps, followups})
	}
	return path, dataset, workHash(common), nil
}

func localFirstTurns(turns []agent.Turn) error {
	if len(turns) == 0 || len(turns) > 8 {
		return errors.New("local-first script must have one through eight turns")
	}
	for _, turn := range turns {
		if len(turn.Text) > 4096 || len(turn.ToolCalls) > 2 || len(turn.ProviderData) != 0 {
			return errors.New("unbounded local-first turn")
		}
		for _, call := range turn.ToolCalls {
			if len(call.Arguments) > 16*1024 || call.Kind != "" {
				return errors.New("invalid local-first tool call")
			}
			var args map[string]json.RawMessage
			if json.Unmarshal(call.Arguments, &args) != nil {
				return errors.New("fixture arguments must be a JSON object")
			}
			switch call.Name {
			case "read_file":
				if len(args) != 1 {
					return errors.New("fixture read requires only path")
				}
				path := string(args["path"])
				if path != `"source/input.json"` && path != `"source/left.txt"` && path != `"source/right.txt"` && path != `"source/.env"` && path != `"source/../../blocked.txt"` && path != "17" {
					return errors.New("fixture read path is not allowlisted")
				}
			case "write_artifact":
				if len(args) != 2 || string(args["path"]) != `"decision.json"` || len(args["content"]) == 0 {
					return errors.New("fixture may write only its decision artifact")
				}
			default:
				return errors.New("local-first reference scripts cannot invoke network, CLI or execution tools")
			}
		}
	}
	return nil
}

type LocalFirstDecision struct {
	Route    string `json:"route"`
	Next     string `json:"next"`
	Complete bool   `json:"complete"`
	Answer   string `json:"answer"`
}

// A small closed schema also rejects duplicate keys. Diagnostics never copy
// raw response values, which could contain a credential in a negative probe.
func decodeLocalFirstDecision(data []byte) (LocalFirstDecision, error) {
	var result LocalFirstDecision
	invalid := errors.New("invalid finite local-first decision schema")
	if len(data) > 16*1024 {
		return result, invalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return result, invalid
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		name, ok := token.(string)
		if err != nil || !ok || seen[name] {
			return result, invalid
		}
		seen[name] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return result, invalid
		}
		switch name {
		case "route":
			err = json.Unmarshal(raw, &result.Route)
		case "next":
			err = json.Unmarshal(raw, &result.Next)
		case "complete":
			err = json.Unmarshal(raw, &result.Complete)
		case "answer":
			err = json.Unmarshal(raw, &result.Answer)
		default:
			return result, invalid
		}
		if err != nil {
			return result, invalid
		}
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || len(seen) != 4 || d.Decode(new(any)) != io.EOF || len(result.Answer) > 8192 {
		return result, invalid
	}
	if !oneOf(result.Route, "local", "cloud", "codex", "claude", "refuse") || !oneOf(result.Next, "answer", "tool", "review", "ask", "retry", "cancel", "refuse") {
		return result, invalid
	}
	return result, nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

type LocalFirstDecisionCheck struct {
	Valid                 bool `json:"valid_schema"`
	RouteCorrect          bool `json:"route_correct"`
	UnnecessaryDelegation bool `json:"unnecessary_delegation"`
	IncorrectCompletion   bool `json:"incorrect_completion"`
	AnswerMatches         bool `json:"answer_matches_reference"`
}

func gradeLocalFirstDecision(g WorkGrader, data []byte) Grade {
	grade := Grade{Kind: g.Kind, Evidence: "finite decision schema and reference fields; no response text copied", LocalFirst: &LocalFirstDecisionCheck{}}
	want, err := decodeLocalFirstDecision([]byte(g.Expected))
	if err != nil {
		grade.Error = "invalid reference decision"
		return grade
	}
	got, err := decodeLocalFirstDecision(data)
	if err != nil {
		return grade
	}
	grade.LocalFirst = &LocalFirstDecisionCheck{Valid: true, RouteCorrect: got.Route == want.Route,
		UnnecessaryDelegation: oneOf(got.Route, "codex", "claude") && !oneOf(want.Route, "codex", "claude"),
		IncorrectCompletion:   got.Complete && !want.Complete, AnswerMatches: got.Answer == want.Answer}
	grade.Passed = got == want
	return grade
}

// WriteLocalFirstSummary supplements the canonical experiment, without
// inventing measurements or interpreting reference transcripts as model quality.
func WriteLocalFirstSummary(directory, commonHash, architecture string, p LocalFirstProfile, r WorkExperiment) error {
	if len(p.digest) != 64 || len(commonHash) != 64 {
		return errors.New("local-first report requires captured input identities")
	}
	if _, ok := p.Arms[architecture]; !ok {
		return errors.New("unknown local-first architecture")
	}
	var requests, latency, decisions, valid, correct, unnecessary, falseCompletion int
	for _, trial := range r.Trials {
		requests += trial.Usage.ModelRequests
		latency += int(trial.DurationMS)
		for _, grade := range trial.Grades {
			if v := grade.LocalFirst; v != nil {
				decisions++
				if v.Valid {
					valid++
				}
				if v.RouteCorrect {
					correct++
				}
				if v.UnnecessaryDelegation {
					unnecessary++
				}
				if v.IncorrectCompletion {
					falseCompletion++
				}
			}
		}
	}
	metadata := map[string]any{
		"version": 1, "profile": p.ID, "profile_sha256": p.digest, "common_inputs_sha256": commonHash,
		"architecture": architecture, "experiment": r.ID, "split": r.Split, "dataset_sha256": r.DatasetSHA256,
		"execution": "deterministic-reference-transcripts", "model_quality": "NOT TESTED", "specialist_execution": "NOT TESTED; decisions only",
		"model": "scripted-reference", "quantization": "not applicable", "runtime": "Go scripted model", "runtime_version": runtime.Version(),
		"host": map[string]string{"os": runtime.GOOS, "architecture": runtime.GOARCH}, "context_tokens_planned": p.ContextTokens, "context_tokens_observed": nil, "concurrency": p.Concurrency,
		"budget":      map[string]any{"requests": p.MaxRequests, "wall_seconds": p.WallSeconds},
		"quality":     map[string]any{"passed": r.Passed, "total": r.Total, "decision_probes": decisions, "valid_decisions": valid, "correct_routes": correct, "unnecessary_delegations": unnecessary, "incorrect_completion_claims": falseCompletion, "semantic_claim_accuracy": nil, "live_recovery_quality": nil, "live_correction_quality": nil},
		"safety":      map[string]any{"provider_requests": 0, "network_requests": 0, "paid_specialist_invocations": 0, "real_downloads": 0, "live_model_permission_compliance": nil, "silent_fallback_requests": 0, "note": "reference script tool allowlist; no live adapter constructed"},
		"performance": map[string]any{"scripted_model_turns": requests, "sum_trial_latency_ms": latency, "first_token_ms": nil, "tokens_per_second": nil, "model_load_ms": nil, "cpu_percent": nil, "gpu_activity": nil, "rss_bytes": nil, "physical_footprint_bytes": nil, "memory_pressure": nil, "swap_bytes": nil, "kv_cache_bytes": nil},
		"cost":        map[string]any{"local_inference_invocations": 0, "paid_invocations": 0, "known_api_cost": nil, "cost_per_successful_live_task": nil, "note": "no billable calls; token prices and energy costs not measured"},
	}
	host := localmodel.DetectHost()
	metadata["host"] = map[string]any{"os": runtime.GOOS, "architecture": runtime.GOARCH, "logical_cpus": runtime.NumCPU(), "total_ram_bytes": host.TotalMemoryBytes, "available_memory_known": host.AvailableMemoryKnown, "memory_measurement_note": host.MemoryError}
	return writeJSON(filepath.Join(directory, "local-first.json"), metadata, "local-first metadata")
}

func LocalFirstProfileSummary(p LocalFirstProfile) string {
	return fmt.Sprintf("%s: four deterministic arms; context plan=%d, concurrency=%d; live inference disabled", p.ID, p.ContextTokens, p.Concurrency)
}
