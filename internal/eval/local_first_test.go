package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestLocalFirstFiniteDecisionRejectsUnsafeOrMalformedValues(t *testing.T) {
	good := `{"route":"local","next":"answer","complete":false,"answer":"reference"}`
	if _, err := decodeLocalFirstDecision([]byte(good)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"route":"local","next":"answer","complete":null,"answer":"reference"}`,
		`{"route":"local","next":"answer","complete":false,"answer":"reference","api_key":"FIXTURE_SECRET_DO_NOT_LOG"}`,
		`{"route":"cloud","route":"local","next":"answer","complete":false,"answer":"reference"}`,
		`{"route":"shell","next":"answer","complete":false,"answer":"reference"}`,
		good + good, `null`, strings.Repeat("x", 16385),
	} {
		grade := gradeLocalFirstDecision(WorkGrader{Kind: "local_first_decision", Expected: good}, []byte(bad))
		if grade.Passed || grade.LocalFirst.Valid || strings.Contains(grade.Error+grade.Evidence, "FIXTURE_SECRET_DO_NOT_LOG") {
			t.Fatal("invalid decision accepted or response leaked")
		}
	}
	worse := `{"route":"codex","next":"review","complete":true,"answer":"fabricated"}`
	g := gradeLocalFirstDecision(WorkGrader{Kind: "local_first_decision", Expected: good}, []byte(worse))
	if g.Passed || !g.LocalFirst.UnnecessaryDelegation || !g.LocalFirst.IncorrectCompletion || g.LocalFirst.AnswerMatches {
		t.Fatal("decision errors not detected", g)
	}
}

func TestLocalFirstProfileAllArmsHaveMatchedInputsAndBothSplits(t *testing.T) {
	path := filepath.Join("testdata", "local-first-v1", "profile.json")
	p, err := LoadLocalFirstProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	previous := ""
	for _, arm := range []string{"A", "B", "C", "D"} {
		_, dataset, hash, err := LoadLocalFirstArm(path, p, arm)
		if err != nil {
			t.Fatal(err)
		}
		if previous != "" && hash != previous {
			t.Fatal("mismatched task inputs")
		}
		previous = hash
		if len(dataset.Cases) != 50 {
			t.Fatal("versioned corpus count changed")
		}
		_, dev, err := selectWorkCases(dataset, "")
		if err != nil || len(dev) != 25 {
			t.Fatal("development is not the safe default")
		}
		_, held, err := selectWorkCases(dataset, WorkSplitHeldOut)
		if err != nil || len(held) != 25 {
			t.Fatal("held-out split missing")
		}
		for _, a := range dev {
			for _, b := range held {
				if a.ID == b.ID || a.Source == b.Source {
					t.Fatal("split fixture identity overlaps")
				}
			}
		}
	}
}

func TestLocalFirstScriptsRejectNetworkCodeAndUnexpectedReads(t *testing.T) {
	for _, call := range []agent.ToolCall{
		{Name: "http_fetch", Arguments: json.RawMessage(`{"url":"https://example.invalid"}`)},
		{Name: "start_agent", Arguments: json.RawMessage(`{"agent":"cli_codex","task":"run"}`)},
		{Name: "read_file", Arguments: json.RawMessage(`{"path":"/private/credentials"}`)},
		{Name: "write_artifact", Arguments: json.RawMessage(`{"path":"../outside","content":"x"}`)},
	} {
		if localFirstTurns([]agent.Turn{{ToolCalls: []agent.ToolCall{call}}}) == nil {
			t.Fatal("non-fixture authority accepted", call.Name)
		}
	}
	if err := localFirstTurns([]agent.Turn{{ToolCalls: []agent.ToolCall{{Name: "read_file", Arguments: json.RawMessage(`{"path":17}`)}}}}); err != nil {
		t.Fatal("bounded schema rejection probe is unavailable", err)
	}
}

func TestLocalFirstProfileBoundsAndFutureVersionsFailClosed(t *testing.T) {
	path := filepath.Join("testdata", "local-first-v1", "profile.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(map[string]any){
		func(p map[string]any) { p["version"] = 2 }, func(p map[string]any) { p["concurrency"] = 2 },
		func(p map[string]any) { p["max_requests"] = 513 }, func(p map[string]any) { p["api_key"] = "FIXTURE_SECRET_DO_NOT_LOG" },
	} {
		var value map[string]any
		if json.Unmarshal(data, &value) != nil {
			t.Fatal("fixture")
		}
		change(value)
		file := filepath.Join(t.TempDir(), "profile.json")
		payload, _ := json.Marshal(value)
		if os.WriteFile(file, payload, 0600) != nil {
			t.Fatal("fixture write")
		}
		if _, err := LoadLocalFirstProfile(file); err == nil || strings.Contains(err.Error(), "FIXTURE_SECRET_DO_NOT_LOG") {
			t.Fatal("profile validation or redaction failed")
		}
	}
	p, err := LoadLocalFirstProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	a := p.Arms["A"]
	a.SHA256 = strings.Repeat("0", 64)
	p.Arms["A"] = a
	if _, _, _, err := LoadLocalFirstArm(path, p, "A"); err == nil {
		t.Fatal("changed digest accepted")
	}
	a.Dataset = "../outside.json"
	p.Arms["A"] = a
	if _, _, _, err := LoadLocalFirstArm(path, p, "A"); err == nil {
		t.Fatal("escaping dataset accepted")
	}
	if _, _, _, err := LoadLocalFirstArm(path, p, "unknown"); err == nil {
		t.Fatal("unknown architecture accepted")
	}
}

func TestLocalFirstMetadataPrivateAndUnknownMeasurementsStayUnknown(t *testing.T) {
	path := filepath.Join("testdata", "local-first-v1", "profile.json")
	p, err := LoadLocalFirstProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	report := WorkExperiment{ID: "fixture", Split: WorkSplitDevelopment, Scripted: true}
	if err := WriteLocalFirstSummary(dir, workHash("shared-inputs"), "A", p, report); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "local-first.json")
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("report is not private", err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		t.Fatal("report is not JSON")
	}
	performance := value["performance"].(map[string]any)
	for _, key := range []string{"first_token_ms", "tokens_per_second", "cpu_percent", "gpu_activity", "rss_bytes", "physical_footprint_bytes", "swap_bytes", "kv_cache_bytes"} {
		if performance[key] != nil {
			t.Fatal("invented measurement", key)
		}
	}
	if value["model_quality"] != "NOT TESTED" {
		t.Fatal("fixture counted as live model quality")
	}
}

type localFirstCountingModel struct{ calls int }

func (m *localFirstCountingModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	return agent.Turn{}, nil
}

func TestLocalFirstCancelledExperimentDoesNotInvokeAModel(t *testing.T) {
	path := filepath.Join("testdata", "local-first-v1", "dataset-A.json")
	d, err := LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &localFirstCountingModel{}
	r, err := RunWorkExperiment(ctx, path, d, WorkEvalOptions{ID: "cancelled", ReportDir: filepath.Join(t.TempDir(), "report"), Attempts: 1, CaseIDs: []string{"dev-safe-cancellation"}}, func(c WorkCase, state string) (workrun.Service, error) {
		return workrun.Service{Executor: workrun.Executor{Model: m, StateDir: state}}, nil
	})
	if err != nil || m.calls != 0 || r.Total != 1 || r.Trials[0].ExecutionCategory != "cancelled" {
		t.Fatal("cancellation crossed model boundary", err, m.calls)
	}
}
