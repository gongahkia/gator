package auth

import (
	"errors"
	"testing"
)

func TestReviewedCredentialChangesRejectStaleLoginRefreshAndDisconnect(t *testing.T) {
	s, _ := New(t.TempDir())
	old := Credential{Type: oauthType, Access: "first", Refresh: "refresh-first"}
	next := Credential{Type: oauthType, Access: "second", Refresh: "refresh-second"}
	if err := s.ReplaceReviewed("account", nil, &old); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceReviewed("account", nil, &next); !errors.Is(err, ErrCredentialChanged) {
		t.Fatal("concurrent first login overwrote a credential", err)
	}
	if err := s.ReplaceReviewed("account", &old, &next); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceReviewed("account", &old, nil); !errors.Is(err, ErrCredentialChanged) {
		t.Fatal("stale disconnect removed the replacement", err)
	}
	if err := s.ReplaceReviewed("account", &next, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceReviewed("account", &next, &old); !errors.Is(err, ErrCredentialChanged) {
		t.Fatal("refresh resurrected a disconnected credential", err)
	}
	if _, found, err := s.Read("account"); err != nil || found {
		t.Fatal("disconnected account returned")
	}
}
