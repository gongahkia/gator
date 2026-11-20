package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/cliagent"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
)

// Audit reproductions describe current gaps, not the desired L2 contract.
// Revisit these expectations when the lifecycle corrections are implemented.
func TestLocalFirstAuditUseOverwritesAConcurrentIndependentWriter(t *testing.T) {
	directory := t.TempDir()
	store, err := config.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	initial := config.Default()
	initial.ExternalAgents = []cliagent.Config{{Kind: "codex", Executable: "/fixture/not-executed-codex", APIKeyEnv: "GATOR_FIXTURE_API_KEY", TimeoutSeconds: 10}}
	if err := store.Save(initial); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"version":"fixture"}`)
			return
		}
		if r.URL.Path == "/api/tags" {
			once.Do(func() {
				close(entered)
				select {
				case <-release:
				case <-r.Context().Done():
				}
			})
			fmt.Fprint(w, `{"models":[{"name":"qwen3:4b","size":2500000000}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	m := newLocalModelManager(store)
	m.runtimeURL = server.URL
	m.host = localFirstAuditHost
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.Use(ctx, "qwen3-4b"); done <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("Use did not reach the controlled read barrier")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestLocalFirstAuditConfigWriterHelper$")
	child.Env = []string{"GATOR_LOCAL_AUDIT_CONFIG=" + directory, "PATH=" + os.Getenv("PATH")}
	output, childErr := child.CombinedOutput()
	close(release)
	if childErr != nil {
		t.Fatal("independent writer", childErr, string(output))
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Theme != "" || len(after.ExternalAgents) != 1 {
		t.Fatal("audit reproduction changed: re-evaluate the lost-update finding")
	}
	t.Log("LF-01 reproduced: independent process saved Theme=mono with Update; stale local Use returned success, reset Theme, and restored the disabled fake CLI configuration")
}

func TestLocalFirstAuditConfigWriterHelper(t *testing.T) {
	directory := os.Getenv("GATOR_LOCAL_AUDIT_CONFIG")
	if directory == "" {
		t.Skip("independent-process helper only")
	}
	s, err := config.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(v *config.Settings) error { v.Theme = "mono"; v.ExternalAgents = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	v, err := s.Load()
	if err != nil || v.Theme != "mono" || len(v.ExternalAgents) != 0 {
		t.Fatal("child write did not persist", err)
	}
}

func localFirstAuditHost() localmodel.Host {
	return localmodel.Host{OS: "darwin", Architecture: "arm64", TotalMemoryBytes: 16 * 1024 * 1024 * 1024, AvailableDiskKnown: true, AvailableDiskBytes: 100 * 1024 * 1024 * 1024}
}

func TestLocalFirstAuditRemovalCanOutliveSettingsPublication(t *testing.T) {
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"version":"fixture"}`)
		case "/api/delete":
			var payload struct{ Model string }
			if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Model != "qwen3:4b" {
				t.Error("wrong removal identity")
			}
			deleted.Store(true)
			if err := os.Rename(store.Path(), store.Path()+".test-backup"); err != nil {
				t.Error(err)
			}
			if err := os.Mkdir(store.Path(), 0700); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s := config.Default()
	s.CustomProviders = []config.CustomProvider{{ID: localmodel.ProviderID, BaseURL: server.URL + "/v1/chat/completions", Models: []string{"qwen3:4b"}, DefaultModel: "qwen3:4b"}}
	s.Defaults.Provider = localmodel.ProviderID
	s.Defaults.Model = "qwen3:4b"
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	m := newLocalModelManager(store)
	m.runtimeURL = server.URL
	m.host = localFirstAuditHost
	_, err = m.Remove(context.Background(), "qwen3-4b")
	if !deleted.Load() || err == nil || !strings.Contains(err.Error(), "removed qwen3:4b but could not update") {
		t.Fatal("removal/publication reproduction changed", err)
	}
	if _, err := os.Stat(filepath.Clean(store.Path() + ".test-backup")); err != nil {
		t.Fatal("original test settings not preserved", err)
	}
	t.Log("LF-02 reproduced: fake Ollama confirmed DELETE, settings replacement failed; operation is partially committed")
}

func TestLocalFirstAuditExistingRuntimeIsNotAdoptedOrStopped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("unexpected mutation")
		}
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"version":"fixture"}`)
		} else {
			fmt.Fprint(w, `{"models":[]}`)
		}
	}))
	defer server.Close()
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := newLocalModelManager(store)
	m.runtimeURL = server.URL
	m.host = localFirstAuditHost
	m.command = func(string, ...string) *exec.Cmd { t.Fatal("existing runtime must not launch a process"); return nil }
	if _, err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.runtime != nil {
		t.Fatal("adopted external runtime")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	c, err := localmodel.NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Version(context.Background()); err != nil {
		t.Fatal("external runtime unavailable after Close", err)
	}
}
