package workrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestEvidenceSearchFindsLateHTMLTermsWithoutPagingWholeSource(t *testing.T) {
	body := strings.Repeat("<a>CheckRedirect index 猫</a>\n", 3000) + "<p>CheckRedirect: verified final policy detail 猫.</p>"
	var evidence artifact.Evidence
	step := 0
	model := researchModel(func(r agent.TurnRequest) (agent.Turn, error) {
		step++
		if step == 1 {
			return researchCall("http_fetch", map[string]any{"url": "https://research.invalid/large"}), nil
		}
		if step == 2 {
			var read struct {
				Evidence artifact.Evidence `json:"evidence"`
			}
			if err := json.Unmarshal([]byte(r.Messages[len(r.Messages)-1].Content), &read); err != nil {
				t.Fatal(err)
			}
			evidence = read.Evidence
			return researchCall("search_evidence", map[string]any{"id": evidence.ID, "query": "CheckRedirect", "limit": 4, "context_bytes": 64}), nil
		}
		result := r.Messages[len(r.Messages)-1].Content
		var found struct {
			Matches []struct {
				Start int    `json:"start_offset_bytes"`
				End   int    `json:"end_offset_bytes"`
				Match int    `json:"match_offset_bytes"`
				Text  string `json:"text"`
			} `json:"matches"`
			Total   int `json:"total_matches"`
			Omitted int `json:"matches_omitted"`
		}
		if err := json.Unmarshal([]byte(result), &found); err != nil {
			t.Fatal(err)
		}
		if len(result) > 4000 || len(found.Matches) != 4 || found.Total != 3001 || found.Omitted != 2997 || !strings.Contains(found.Matches[3].Text, "verified final policy") {
			t.Fatal("evidence search missed late content or inflated context", result)
		}
		for _, match := range found.Matches {
			if !utf8.ValidString(match.Text) || body[match.Start:match.End] != match.Text || body[match.Match:match.Match+len("CheckRedirect")] != "CheckRedirect" {
				t.Fatal("search offsets did not identify exact retained bytes")
			}
		}
		return agent.Turn{Text: "Located the final policy detail in bounded exact evidence."}, nil
	})
	out, err := (Executor{Model: model, StateDir: t.TempDir(), HTTP: tools.HTTPFetchOptions{Client: htmlResearchHTTP{body}, Resolver: researchResolver{}}}).Execute(context.Background(), Request{SourcePath: t.TempDir(), Objective: "Find a detail in a large primary page", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 4, WebOrigins: []string{"https://research.invalid"}})
	if err != nil {
		t.Fatal(err)
	}
	catalog := &evidenceCatalog{work: out.Work, entries: map[string]artifact.Evidence{evidence.ID: evidence}}
	tool := evidenceSearchTool{catalog}
	for _, raw := range []string{`{"id":"unknown","query":"CheckRedirect"}`, `{"id":"` + evidence.ID + `","query":"x","offset_bytes":-1}`, `{"id":"` + evidence.ID + `","query":"x","context_bytes":2048}`, `{"id":"` + evidence.ID + `","query":"x","remote_url":"https://other.invalid"}`} {
		if _, err := tool.Execute(context.Background(), []byte(raw)); err == nil {
			t.Fatal("invalid or out-of-scope evidence search succeeded", raw)
		}
	}
	if err := os.WriteFile(filepath.Join(out.Work.Path, evidence.SnapshotPath), []byte("tampered source"), 0600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"id": evidence.ID, "query": "CheckRedirect"})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("changed retained bytes accepted")
	}
}

func TestConcurrentResearchRetainsCompleteContentAddressedEvidence(t *testing.T) {
	catalog := &evidenceCatalog{work: workspace.Work{Path: t.TempDir()}, entries: map[string]artifact.Evidence{}}
	data := []byte(strings.Repeat("Public source 猫\n", 10000))
	var wg sync.WaitGroup
	errorsOut := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				entry, err := catalog.retain("https://research.invalid/shared", data, time.Now())
				if err != nil {
					errorsOut <- err
					return
				}
				_, read, err := catalog.read(entry.ID)
				if err != nil {
					errorsOut <- err
					return
				}
				if string(read) != string(data) {
					t.Error("concurrent researcher observed a partial content-addressed file")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errorsOut)
	for err := range errorsOut {
		t.Error(err)
	}
}
