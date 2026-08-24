package reviewweb

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/review"
)

func TestBrowserBootstrapIsOneUseAndKeepsSecretsOutOfThePageURL(t *testing.T) {
	server, _ := reviewServerFixture(t)
	web := httptest.NewServer(server.Handler())
	defer web.Close()
	bootstrap, err := server.BootstrapURL(strings.TrimPrefix(web.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	response, err := client.Get(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	contents, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || strings.Contains(response.Request.URL.String(), "access=") || !strings.Contains(string(contents), "Gator review") {
		t.Fatalf("bootstrap response status=%d final=%q body=%q", response.StatusCode, response.Request.URL, contents)
	}
	if csp := response.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "script-src 'nonce-") {
		t.Fatalf("page CSP = %q", csp)
	}
	response, err = client.Get(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second bootstrap status = %d", response.StatusCode)
	}
}

func TestBrowserReviewStagesOneHunkAndSavesServerDerivedFeedback(t *testing.T) {
	server, record := reviewServerFixture(t)
	web := httptest.NewServer(server.Handler())
	defer web.Close()
	client := authenticatedReviewClient(t, server, web.URL)
	snapshot := getSnapshot(t, client, web.URL)
	file := onlyReviewFile(t, snapshot.Unstaged)
	if len(file.Hunks) != 1 {
		fatalf(t, "unstaged hunks = %d", len(file.Hunks))
	}
	payload, _ := json.Marshal(stageRequest{Scope: review.Unstaged, FileID: file.ID, HunkID: file.Hunks[0].ID})
	request, err := http.NewRequest(http.MethodPost, web.URL+"/review/api/stage", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", web.URL)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stage status = %d", response.StatusCode)
	}
	updated := getSnapshot(t, client, web.URL)
	stagedFile := onlyReviewFile(t, updated.Staged)
	if len(stagedFile.Hunks) != 1 {
		t.Fatalf("staged hunks = %d", len(stagedFile.Hunks))
	}
	feedbackPayload, _ := json.Marshal(feedbackRequest{Scope: review.Staged, FileID: stagedFile.ID, HunkID: stagedFile.Hunks[0].ID, Start: 0, End: 1, Instruction: "Keep the behavior and add a focused regression test."})
	request, err = http.NewRequest(http.MethodPost, web.URL+"/review/api/feedback", bytes.NewReader(feedbackPayload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", web.URL)
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	contents, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusCreated || !strings.Contains(string(contents), "untrusted code context") {
		t.Fatalf("feedback status=%d body=%s", response.StatusCode, contents)
	}
	feedback, err := journal.ListReviewFeedback(record.StatePath)
	if err != nil || len(feedback) != 1 || feedback[0].File != "alpha.txt" || len(feedback[0].Before) == 0 || len(feedback[0].After) == 0 {
		t.Fatalf("saved feedback = %#v, %v", feedback, err)
	}
}

func TestBrowserReviewRejectsRemoteAndCrossOriginMutation(t *testing.T) {
	server, _ := reviewServerFixture(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/review/", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("remote status = %d", response.Code)
	}

	web := httptest.NewServer(server.Handler())
	defer web.Close()
	client := authenticatedReviewClient(t, server, web.URL)
	request, err := http.NewRequest(http.MethodPost, web.URL+"/review/api/stage", strings.NewReader(`{"scope":"unstaged","file_id":"x","whole":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://untrusted.example")
	request.Header.Set("Content-Type", "application/json")
	responseHTTP, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseHTTP.Body.Close()
	if responseHTTP.StatusCode != http.StatusUnauthorized {
		t.Fatalf("cross-origin mutation status = %d", responseHTTP.StatusCode)
	}
}

func reviewServerFixture(t *testing.T) (*Server, journal.Record) {
	t.Helper()
	repository := t.TempDir()
	gitOK(t, repository, "init", "--quiet")
	gitOK(t, repository, "config", "user.name", "Gator Test")
	gitOK(t, repository, "config", "user.email", "gator@example.invalid")
	writeFile(t, repository, "alpha.txt", "one\ntwo\nthree\n")
	gitOK(t, repository, "add", ".")
	gitOK(t, repository, "commit", "--quiet", "-m", "base")
	writeFile(t, repository, "alpha.txt", "one\ntwo changed\nthree\n")
	base := strings.TrimSpace(gitOutput(t, repository, "rev-parse", "HEAD"))
	entry, record, err := journal.Open(repository, "run-review-web-001", repository, t.TempDir(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	defer entry.Close()
	if err := entry.SaveSession(journal.Session{Version: 2, Repository: repository, WorktreePath: repository, BaseCommit: base, Provider: "openai", Model: "test", Task: "Review web fixture"}); err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{StatePath: record.StatePath})
	if err != nil {
		t.Fatal(err)
	}
	return server, record
}

func authenticatedReviewClient(t *testing.T, server *Server, baseURL string) *http.Client {
	t.Helper()
	bootstrap, err := server.BootstrapURL(strings.TrimPrefix(baseURL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	response, err := client.Get(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status = %d", response.StatusCode)
	}
	return client
}

func getSnapshot(t *testing.T, client *http.Client, baseURL string) review.Snapshot {
	t.Helper()
	response, err := client.Get(baseURL + "/review/api/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("snapshot status = %d", response.StatusCode)
	}
	var snapshot review.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func onlyReviewFile(t *testing.T, set review.ChangeSet) review.File {
	t.Helper()
	if len(set.Files) != 1 {
		t.Fatalf("review files = %#v", set.Files)
	}
	return set.Files[0]
}

func writeFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitOK(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.CommandContext(context.Background(), "git", arguments...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.CommandContext(context.Background(), "git", arguments...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(arguments, " "), err)
	}
	return string(output)
}

func fatalf(t *testing.T, format string, values ...any) { t.Helper(); t.Fatalf(format, values...) }
