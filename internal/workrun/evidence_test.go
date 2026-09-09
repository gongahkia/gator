package workrun

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/tools"
)

type researchResolver struct{}

func (researchResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

type researchHTTP struct{ calls int }

func (c *researchHTTP) Do(r *http.Request) (*http.Response, error) {
	c.calls++
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: io.NopCloser(strings.NewReader("Revenue is 120.")), Request: r}, nil
}

type researchModel func(agent.TurnRequest) (agent.Turn, error)

func (m researchModel) Complete(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
	return m(r)
}
func researchCall(name string, args any) agent.Turn {
	data, _ := json.Marshal(args)
	return agent.Turn{ToolCalls: []agent.ToolCall{{ID: name, Name: name, Arguments: data}}}
}
func TestResearchBriefRetainsLocalWebConnectedEvidenceAndFollowup(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "local.txt"), []byte("Revenue is 100."), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"revenue":110}`)
	}))
	defer server.Close()
	registry, err := connector.NewRegistry([]connector.Descriptor{{Version: connector.DescriptorVersion, ID: "metrics", Name: "Metrics", Kind: connector.KindHTTPJSON, Resource: server.URL, Authentication: connector.AuthNone}})
	if err != nil {
		t.Fatal(err)
	}
	web := &researchHTTP{}
	step := 0
	var localID, webID string
	model := researchModel(func(r agent.TurnRequest) (agent.Turn, error) {
		step++
		switch step {
		case 1:
			return researchCall("http_fetch", map[string]string{"url": "https://unselected.invalid/report"}), nil
		case 2:
			return researchCall("read_file", map[string]string{"path": "source/local.txt"}), nil
		case 3:
			return researchCall("http_fetch", map[string]string{"url": "https://research.invalid/report"}), nil
		case 4:
			return researchCall("connector_metrics_fetch", map[string]any{}), nil
		case 5:
			return researchCall("list_evidence", map[string]any{}), nil
		case 6:
			text := r.Messages[len(r.Messages)-1].Content
			var envelope struct {
				Entries []artifact.Evidence `json:"entries"`
			}
			if err := json.Unmarshal([]byte(text), &envelope); err != nil {
				t.Fatal(err)
			}
			for _, entry := range envelope.Entries {
				if entry.Locator == "source/local.txt" {
					localID = entry.ID
				}
				if entry.Locator == "https://research.invalid/report" {
					webID = entry.ID
				}
			}
			if localID == "" || webID == "" {
				t.Fatalf("missing catalog IDs: %s", text)
			}
			return researchCall("check_claims", map[string]any{"claims": []any{map[string]string{"claim": "conflicting revenue", "evidence_id": localID, "quote": "Revenue is 100."}, map[string]string{"claim": "conflicting revenue", "evidence_id": webID, "quote": "Revenue is 120."}, map[string]string{"claim": "unjustified agreement", "evidence_id": webID, "quote": "Revenue is 100."}}}), nil
		case 7:
			return researchCall("write_artifact", map[string]string{"path": "report.md", "content": "Revenue is 100. [" + localID + "] Revenue is 120. [" + webID + "] Connected metrics report 110. The sources conflict; no authoritative resolution is available."}), nil
		default:
			return agent.Turn{Text: "Brief retains conflicting sources and unsupported quote check."}, nil
		}
	})
	service := Service{Executor: Executor{Model: model, StateDir: t.TempDir(), HTTP: tools.HTTPFetchOptions{Client: web, Resolver: researchResolver{}}, Connectors: connector.Runtime{Registry: registry, HTTPClient: server.Client()}}}
	request := Request{SourcePath: source, Objective: "Draft a cited brief; preserve uncertainty", Contract: artifact.DefaultContract("report.md"), WebOrigins: []string{"https://research.invalid"}, ConnectorIDs: []string{"metrics"}}
	first, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if web.calls != 1 || len(first.Manifest.Evidence) != 3 || len(first.Manifest.ConnectedSources) != 1 {
		t.Fatalf("evidence=%+v calls=%d", first.Manifest.Evidence, web.calls)
	}
	unsupported := false
	for _, event := range first.Events {
		unsupported = unsupported || strings.Contains(event.ToolResult, `"quote_match":false`)
	}
	if !unsupported {
		t.Fatal("unsupported quote not recorded")
	}
	if err := os.WriteFile(filepath.Join(source, "local.txt"), []byte("changed live input"), 0600); err != nil {
		t.Fatal(err)
	}
	request.ConversationID = first.ConversationID
	request.Objective = "Shorten the brief, retaining uncertainty"
	service.Executor.Model = &scriptedModel{turns: []agent.Turn{researchCall("write_artifact", map[string]string{"path": "report.md", "content": "The selected sources conflict: 100 [" + localID + "] versus 120 [" + webID + "]. Resolution remains unknown."}), {Text: "Retained citations and uncertainty."}}}
	second, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if web.calls != 1 || len(second.Manifest.Evidence) != 3 {
		t.Fatal("followup lost evidence or silently fetched again")
	}
	found := false
	for _, entry := range second.Manifest.Evidence {
		found = found || entry.ID == localID
	}
	if !found {
		t.Fatal("stable evidence reference changed")
	}
	bundle, err := artifact.OpenBundle(second.Work.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		t.Fatal(err)
	}
	entry := second.Manifest.Evidence[0]
	if err := os.WriteFile(filepath.Join(second.Work.Path, entry.SnapshotPath), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := artifact.VerifyBundle(bundle); err == nil {
		t.Fatal("tampered retained evidence verified")
	}
}
