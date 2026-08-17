package auth

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestStorePutReadDeleteUsesPrivateFile(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	want := Credential{Type: apiKeyType, Key: "test-key", Extra: map[string]string{"region": "test"}}
	if err := store.Put("openai", want); err != nil {
		t.Fatalf("put credential: %v", err)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("stat credential file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("credential mode = %o, want 600", got)
	}
	credential, found, err := store.Read("OpenAI")
	if err != nil || !found {
		t.Fatalf("read credential = %#v, found=%v, error=%v", credential, found, err)
	}
	if !reflect.DeepEqual(credential, want) {
		t.Fatalf("credential = %#v, want %#v", credential, want)
	}
	credential.Extra["region"] = "mutated"
	again, found, err := store.Read("openai")
	if err != nil || !found || again.Extra["region"] != "test" {
		t.Fatalf("stored credential was mutated: %#v, found=%v, error=%v", again, found, err)
	}
	if err := store.Delete("openai"); err != nil {
		t.Fatalf("delete credential: %v", err)
	}
	if _, found, err := store.Read("openai"); err != nil || found {
		t.Fatalf("read after delete found=%v, error=%v", found, err)
	}
}

func TestStoreRejectsInvalidAndUnsafeCredentials(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	for _, credential := range []Credential{
		{Type: apiKeyType},
		{Type: bearerTokenType},
		{Type: bearerTokenType, Access: "token", Key: "not-allowed"},
		{Type: bearerTokenType, Access: "token", Refresh: "not-allowed"},
		{Type: bearerTokenType, Access: "token", Expires: -1},
		{Type: oauthType, Access: "token", Key: "not-allowed"},
		{Type: oauthType, Access: "token", Expires: -1},
		{Type: "unknown", Key: "key"},
	} {
		if err := store.Put("openai", credential); err == nil {
			t.Fatalf("invalid credential %#v was accepted", credential)
		}
	}
	if err := store.Put("../openai", Credential{Type: apiKeyType, Key: "key"}); err == nil {
		t.Fatal("unsafe provider was accepted")
	}
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatalf("create auth directory: %v", err)
	}
	if err := os.Symlink("/dev/null", store.Path()); err != nil {
		t.Fatalf("create credential symlink: %v", err)
	}
	if _, _, err := store.Read("openai"); err == nil {
		t.Fatal("credential symlink was accepted")
	}
}

func TestOAuthCredentialExpiry(t *testing.T) {
	credential := Credential{Type: oauthType, Access: "access", Refresh: "refresh", Expires: time.Now().Add(-time.Second).UnixMilli()}
	if !credential.Expired(time.Now()) {
		t.Fatal("expired OAuth credential was treated as current")
	}
	credential.Expires = 0
	if credential.Expired(time.Now()) {
		t.Fatal("non-expiring OAuth credential was treated as expired")
	}
}

func TestBearerTokenCredentialExpiry(t *testing.T) {
	credential := Credential{Type: bearerTokenType, Access: "access", Expires: time.Now().Add(-time.Second).UnixMilli()}
	if !credential.Expired(time.Now()) {
		t.Fatal("expired bearer token was treated as current")
	}
	credential.Expires = 0
	if credential.Expired(time.Now()) {
		t.Fatal("non-expiring bearer token was treated as expired")
	}
}
