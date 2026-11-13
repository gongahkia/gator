package worksession

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFailedRecoveryEvidenceFailureDoesNotMutateOriginalRequest(t *testing.T) {
	s, id, _ := requestTestStore(t)
	r, err := NewRequest("Read original input", json.RawMessage(`{"Mode":"inspect","MaxSteps":24}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := s.ExecutionBoundary(id, r)
	if err != nil {
		t.Fatal(err)
	}
	r.ExecutionBoundary = &boundary
	r.State = "failed"
	r.WorkID = "missing-original-run"
	r.RevisionID = "missing-original-revision"
	r.Recovery = &RequestRecovery{Kind: "credentials", Status: 401, Summary: "Provider rejected credentials", Repair: "Repair original provider credentials", RetrySafe: true, ScopeDigest: strings.Repeat("a", 64)}
	if _, err := s.SaveRequests(id, RequestJournal{Version: 1, Requests: []Request{r}}); err != nil {
		t.Fatal(err)
	}
	turn := editMessage(t, s, id, r.ID, "recovery-review-0001")
	before, _ := s.LoadRequests(id)
	rawBefore, _ := json.Marshal(before)
	if _, _, err := s.PrepareFailedRequestReview(id, r.ID, id, turn.ID, turn.Text); err == nil {
		t.Fatal("missing immutable evidence admitted review")
	}
	after, _ := s.LoadRequests(id)
	rawAfter, _ := json.Marshal(after)
	if !bytes.Equal(rawBefore, rawAfter) {
		t.Fatal("failed accounting mutated original request")
	}
	if !after.Requests[0].AwaitingRecovery() || after.Requests[0].Pending() {
		t.Fatal("failed recovery became runnable")
	}
}
