package action

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestModeAuthorityIsMonotonic(t *testing.T) {
	t.Parallel()

	readCapabilities := []Capability{SourceRead, NetworkRead, ConnectedRead}
	for _, capability := range readCapabilities {
		if !Inspect.Allows(capability) || !Draft.Allows(capability) || !Act.Allows(capability) {
			t.Fatalf("read capability %q is not available monotonically", capability)
		}
	}
	for _, capability := range []Capability{ArtifactWrite, ProcessExecute} {
		if Inspect.Allows(capability) || !Draft.Allows(capability) || !Act.Allows(capability) {
			t.Fatalf("draft capability %q has unexpected authority", capability)
		}
	}
	for _, capability := range []Capability{ConnectedMutate, Publish} {
		if Inspect.Allows(capability) || Draft.Allows(capability) || !Act.Allows(capability) || !RequiresFreshApproval(capability) {
			t.Fatalf("act capability %q has unexpected authority", capability)
		}
	}
	if Inspect.Allows(Capability("unknown")) {
		t.Fatal("unknown capability was allowed")
	}
}

func TestMeetChoosesLeastAuthority(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		left, right Mode
		want        Mode
	}{
		{Act, Draft, Draft},
		{Inspect, Act, Inspect},
		{Draft, Draft, Draft},
	} {
		got, err := Meet(test.left, test.right)
		if err != nil || got != test.want {
			t.Fatalf("Meet(%q, %q) = %q, %v; want %q", test.left, test.right, got, err, test.want)
		}
	}
	if _, err := Meet(Mode("unknown"), Draft); err == nil {
		t.Fatal("unknown mode was accepted")
	}
}

func TestProposalBindsReviewToPayload(t *testing.T) {
	t.Parallel()

	proposal, err := NewProposal(
		"action-1", Publish, "email", "send", "person@example.com",
		"Send the reviewed weekly brief to person@example.com", []byte("exact private payload"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.PayloadSHA256 != "e81b55f10ccea260914e9382b11c73a888549320f51be3dd15d6431dc6d60068" {
		t.Fatalf("payload digest = %q", proposal.PayloadSHA256)
	}
	if err := proposal.Validate(); err != nil {
		t.Fatal(err)
	}

	proposal.Preview = strings.Repeat("x", maxPreviewBytes+1)
	if err := proposal.Validate(); err == nil {
		t.Fatal("oversized preview was accepted")
	}
}

func TestProposalRejectsReadOnlyAndUnsafeEnvelopes(t *testing.T) {
	t.Parallel()

	if _, err := NewProposal("action-1", ConnectedRead, "drive", "read", "document", "Read a document", nil); err == nil {
		t.Fatal("read-only operation became an external action proposal")
	}
	if _, err := NewProposal("action-1\nforged", Publish, "email", "send", "person@example.com", "Send message", nil); err == nil {
		t.Fatal("unsafe proposal identifier was accepted")
	}
}

func TestBrokerDraftsWithoutApprovalOrExecution(t *testing.T) {
	t.Parallel()
	proposal, err := NewProposal("action-1", Publish, "release", "publish", "https://example.com/hook", `{\"title\":\"Draft\"}`, []byte(`{\"title\":\"Draft\"}`))
	if err != nil {
		t.Fatal(err)
	}
	approved, executed := false, false
	record, err := (Broker{Approve: func(context.Context, Proposal) (Decision, error) {
		approved = true
		return Allow, nil
	}}).Resolve(context.Background(), Draft, Propose, proposal, func(context.Context) error {
		executed = true
		return nil
	})
	if err != nil || record.Status != Pending || approved || executed {
		t.Fatalf("draft record = %#v, err = %v, approved = %t, executed = %t", record, err, approved, executed)
	}
}

func TestBrokerRequiresFreshApprovalForEachExecution(t *testing.T) {
	t.Parallel()
	proposal, err := NewProposal("action-1", ConnectedMutate, "tracker", "create", "https://example.com/issues", "Create issue", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	approvals, executions := 0, 0
	broker := Broker{Approve: func(_ context.Context, got Proposal) (Decision, error) {
		approvals++
		if got.PayloadSHA256 != proposal.PayloadSHA256 {
			t.Fatal("approver received a different payload digest")
		}
		return Allow, nil
	}}
	for range 2 {
		record, err := broker.Resolve(context.Background(), Act, Approve, proposal, func(context.Context) error {
			executions++
			return nil
		})
		if err != nil || record.Status != Executed {
			t.Fatalf("execution record = %#v, err = %v", record, err)
		}
	}
	if approvals != 2 || executions != 2 {
		t.Fatalf("approvals = %d, executions = %d", approvals, executions)
	}
}

func TestBrokerRecordsDenialAndBoundedFailure(t *testing.T) {
	t.Parallel()
	proposal, err := NewProposal("action-1", Publish, "release", "publish", "https://example.com/hook", "Publish release", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	denied, err := (Broker{Approve: func(context.Context, Proposal) (Decision, error) { return Deny, nil }}).
		Resolve(context.Background(), Act, Approve, proposal, func(context.Context) error { return errors.New("must not run") })
	if err != nil || denied.Status != Denied {
		t.Fatalf("denied record = %#v, err = %v", denied, err)
	}
	failed, err := (Broker{Approve: func(context.Context, Proposal) (Decision, error) { return Allow, nil }}).
		Resolve(context.Background(), Act, Approve, proposal, func(context.Context) error {
			return errors.New(strings.Repeat("failure", 200) + "\x00")
		})
	if err != nil || failed.Status != Failed || len(failed.Error) > 512 || strings.ContainsRune(failed.Error, 0) {
		t.Fatalf("failed record = %#v, err = %v", failed, err)
	}
}

func TestBrokerPreservesUncertainExecutionOutcome(t *testing.T) {
	t.Parallel()
	proposal, err := NewProposal("action-1", Publish, "release", "publish", "https://example.com/hook", "Publish release", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	record, err := (Broker{Approve: func(context.Context, Proposal) (Decision, error) { return Allow, nil }}).
		Resolve(context.Background(), Act, Approve, proposal, func(context.Context) error {
			return MarkUncertain(errors.New("connection closed while awaiting response"))
		})
	if err != nil || record.Status != Unknown || record.Error == "" {
		t.Fatalf("uncertain record = %#v, err = %v", record, err)
	}
}

func TestBrokerRejectsAuthorityGaps(t *testing.T) {
	t.Parallel()
	proposal, err := NewProposal("action-1", Publish, "release", "publish", "https://example.com/hook", "Publish release", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		mode        Mode
		disposition Disposition
	}{
		{Inspect, Propose},
		{Draft, Approve},
		{Act, Forbid},
	} {
		if _, err := (Broker{}).Resolve(context.Background(), test.mode, test.disposition, proposal, func(context.Context) error { return nil }); err == nil {
			t.Fatalf("mode %q disposition %q was accepted", test.mode, test.disposition)
		}
	}
}
