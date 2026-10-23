package worksession

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
)

func requestTestStore(t *testing.T) (Store, string, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateChat(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return s, c.ID, dir
}

func retainedRequest(t *testing.T, text string) Request {
	t.Helper()
	r, err := NewRequest(text, json.RawMessage(`{"Uploads":["opaque-file"],"Mode":"inspect","MaxSteps":24}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRequestAdmissionSurvivesReopenAndRejectsStaleChanges(t *testing.T) {
	s, id, dir := requestTestStore(t)
	journal, err := s.LoadRequests(id)
	if err != nil || journal.Version != 1 || journal.Generation != 0 {
		t.Fatalf("new journal: %+v %v", journal, err)
	}
	first, second := retainedRequest(t, "first"), retainedRequest(t, "second")
	journal.Requests = []Request{first, second}
	saved, err := s.SaveRequests(id, journal)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Generation != 1 {
		t.Fatal("admission did not advance generation")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.LoadRequests(id)
	loadedJSON, _ := json.Marshal(loaded)
	savedJSON, _ := json.Marshal(saved)
	if err != nil || string(loadedJSON) != string(savedJSON) {
		t.Fatalf("lost admitted request: %+v %v", loaded, err)
	}
	loaded.Requests[0], loaded.Requests[1] = loaded.Requests[1], loaded.Requests[0]
	loaded.Requests[0].Text, loaded.Requests[0].Edit = "corrected second", 2
	loaded.Requests[1].State = "running"
	if _, err := reopened.SaveRequests(id, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveRequests(id, saved); !errors.Is(err, ErrRequestsChanged) {
		t.Fatalf("stale UI resurrected request: %v", err)
	}
	got, err := s.LoadRequests(id)
	if err != nil || got.Requests[0].ID != second.ID || got.Requests[0].Text != "corrected second" || got.Requests[1].State != "running" {
		t.Fatalf("order/edit/start not retained: %+v %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(s.root, id, "requests.json"))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("request permissions: %v %v", info, err)
		}
	}
}

func TestRequestConcurrentAdmissionHasOneWinner(t *testing.T) {
	s, id, _ := requestTestStore(t)
	var group sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		r := retainedRequest(t, "admit")
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := s.SaveRequests(id, RequestJournal{Version: 1, Requests: []Request{r}})
			results <- err
		}()
	}
	group.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrRequestsChanged) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("%d admissions replaced one another", winners)
	}
}

func TestRequestJournalRejectsCorruptionAndUnsafeInputs(t *testing.T) {
	s, id, _ := requestTestStore(t)
	r := retainedRequest(t, "safe")
	for _, mutate := range []func(*Request){
		func(r *Request) { r.ID = "../escape" }, func(r *Request) { r.State = "retry automatically" },
		func(r *Request) { r.Options = json.RawMessage(`[]`) }, func(r *Request) { r.Text = strings.Repeat("x", 128*1024+1) },
		func(r *Request) { r.UpdatedAt = time.Time{} }, func(r *Request) { r.Edit = 0 },
	} {
		bad := r
		mutate(&bad)
		if _, err := s.SaveRequests(id, RequestJournal{Version: 1, Requests: []Request{bad}}); err == nil {
			t.Fatal("accepted invalid request")
		}
	}
	if _, err := s.SaveRequests(id, RequestJournal{Version: 1, Requests: []Request{r, r}}); err == nil {
		t.Fatal("accepted duplicate identity")
	}
	for _, payload := range []string{`{broken`, `{"version":1,"unknown":true}`, `{"version":1,"requests":[]} {}`} {
		if err := os.WriteFile(filepath.Join(s.root, id, "requests.json"), []byte(payload), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.LoadRequests(id); err == nil {
			t.Fatal("corrupt journal silently became empty")
		}
		if _, err := s.SaveRequests(id, RequestJournal{Version: 1}); err == nil {
			t.Fatal("overwrote corrupt admission evidence")
		}
	}
	if _, err := s.LoadRequests("../escape"); err == nil {
		t.Fatal("accepted unsafe conversation")
	}
}

func TestExecutionOwnerIsExclusiveAndReleasedOnProcessExit(t *testing.T) {
	s, id, dir := requestTestStore(t)
	lease, err := s.AcquireExecution(id)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	probe := func(want string) {
		t.Helper()
		command := exec.Command(os.Args[0], "-test.run=^TestRequestOwnerChild$")
		command.Env = append(os.Environ(), "GATOR_REQUEST_TEST_ROOT="+dir, "GATOR_REQUEST_TEST_CHAT="+id, "GATOR_REQUEST_TEST_WANT="+want)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("execution child: %v\n%s", err, out)
		}
	}
	probe("busy")
	other, err := s.CreateChat(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	independent, err := s.AcquireExecution(other.ID)
	if err != nil {
		t.Fatal("another conversation was blocked", err)
	}
	independent.Close()
	lease.Close()
	probe("available")
	lease, err = s.AcquireExecution(id)
	if err != nil {
		t.Fatal("process exit did not release ownership", err)
	}
	lease.Close()
}

func TestRequestOwnerChild(t *testing.T) {
	root := os.Getenv("GATOR_REQUEST_TEST_ROOT")
	if root == "" {
		return
	}
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := s.AcquireExecution(os.Getenv("GATOR_REQUEST_TEST_CHAT"))
	if os.Getenv("GATOR_REQUEST_TEST_WANT") == "busy" {
		if !errors.Is(err, filelock.ErrWouldBlock) {
			t.Fatalf("expected live owner: %v", err)
		}
	} else {
		if err != nil {
			t.Fatal(err)
		}
		// Deliberately retain the handle until process exit.
		runtime.KeepAlive(lease)
	}
}

func TestAdmittedRequestNamesChatBeforeAnyCompletedRevision(t *testing.T) {
	s, id, dir := requestTestStore(t)
	request := retainedRequest(t, "  Send the reviewed\nvenue update  ")
	journal, err := s.SaveRequests(id, RequestJournal{Version: 1, Requests: []Request{request}})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := reopened.Load(id)
	if err != nil || chat.Title != "Send the reviewed venue update" || chat.HeadRevision != "" {
		t.Fatal("interrupted first task is indistinguishable from an empty chat", chat, err)
	}
	journal.Requests[0].Text = "A later edit must not rename the chat"
	if _, err = s.SaveRequests(id, journal); err != nil {
		t.Fatal(err)
	}
	chat, err = s.Load(id)
	if err != nil || chat.Title != "Send the reviewed venue update" {
		t.Fatal("later queue edit replaced the name", chat, err)
	}
	for _, text := range []string{strings.Repeat("界", 200), strings.Repeat("🐊", 200), strings.Repeat("a", 100)} {
		title := chatRequestTitle(text)
		if len(title) > 256 || len([]rune(title)) > 80 || !strings.HasSuffix(title, "...") {
			t.Fatal("unbounded chat title", title)
		}
	}
}
