package action

import (
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
