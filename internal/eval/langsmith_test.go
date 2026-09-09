package eval

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLangSmithAssociationsRedactionAndErrors(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "SECRET") || r.Header.Get("x-api-key") != "test-key" {
			t.Error("invalid credential/content boundary")
		}
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		switch r.URL.Path {
		case "/datasets":
			io.WriteString(w, `{"id":"dataset"}`)
		case "/examples":
			if value["dataset_id"] != "dataset" {
				t.Error("example dataset association")
			}
			io.WriteString(w, `{"id":"example"}`)
		case "/sessions":
			if value["reference_dataset_id"] != "dataset" {
				t.Error("experiment dataset association")
			}
			io.WriteString(w, `{"id":"experiment"}`)
		case "/runs":
			if value["session_id"] != "experiment" || value["reference_example_id"] != "example" {
				t.Error("run associations")
			}
		case "/feedback":
			if value["session_id"] != "experiment" || value["run_id"] == "" || value["score"] != float64(1) {
				t.Error("feedback association")
			}
		default:
			t.Error("unexpected request")
		}
	}))
	defer server.Close()
	report := WorkExperiment{Version: 1, ID: "experiment", Dataset: "corpus", Scripted: true, Trials: []WorkTrial{{ID: "trial", CaseID: "case", CaseSHA256: "digest", Status: "passed", Error: "SECRET_ERROR", Grades: []Grade{{Kind: "file_equals", Passed: true, Evidence: "SECRET_ORACLE"}}}}}
	if err := ExportLangSmith(context.Background(), server.Client(), server.URL, "test-key", report); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 6 {
		t.Fatal(calls)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer bad.Close()
	if err := ExportLangSmith(context.Background(), bad.Client(), bad.URL, "test-key", report); err == nil {
		t.Fatal("export error ignored")
	}
}
