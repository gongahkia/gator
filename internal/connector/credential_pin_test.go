package connector

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
)

func TestReviewedBearerIdentityIsCheckedBeforeEveryRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: 1, ID: "reports", Name: "Reports", Kind: KindHTTPJSON, Resource: server.URL, Authentication: AuthBearer}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	credential := auth.Credential{Type: "bearer_token", Access: "controlled-original"}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(credential)
	pin := fmt.Sprintf("%x", sha256.Sum256(raw))
	runtime := Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client(), CredentialPins: map[string]string{"reports": pin}}
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "reports", "fetch", json.RawMessage(`{}`)); err != nil || calls != 1 {
		t.Fatal("reviewed identity was not used", err, calls)
	}
	credential.Access = "controlled-replacement"
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Invoke(context.Background(), action.Inspect, "reports", "fetch", json.RawMessage(`{}`))
	var access AccessError
	if !errors.As(err, &access) || access.Kind != "changed_account" || calls != 1 {
		t.Fatal("changed account reached the source", err, calls)
	}
	descriptor.Authentication = AuthNone
	registry, err = NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Registry = registry
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "reports", "fetch", json.RawMessage(`{}`)); err == nil || calls != 1 {
		t.Fatal("authentication downgrade bypassed reviewed identity", err, calls)
	}
}
