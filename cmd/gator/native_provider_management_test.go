package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/modelcatalog"
)

const nativeFixtureSecret = "P04_INERT_SECRET_NOT_A_REAL_KEY_8b7d2"

func nativeProviderFixture(t *testing.T) *nativeProviderBackend {
	t.Helper()
	for _, name := range model.Names() {
		p, _ := model.ParseProvider(name)
		t.Setenv(model.APIKeyEnvironment(p), "")
	}
	root := t.TempDir()
	s, err := config.New(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Save(config.Default()); err != nil {
		t.Fatal(err)
	}
	return &nativeProviderBackend{settings: s, stateDir: filepath.Join(root, "state")}
}
func providerReview(t *testing.T, b *nativeProviderBackend, replace bool, endpoint string) modelcatalog.NativeProviderReview {
	t.Helper()
	r := b.Apply(modelcatalog.NativeProviderAction{RequestID: "review", Action: "review", Provider: "openai", ReplaceKey: replace, Input: &modelcatalog.NativeProviderInput{Model: "fixture-model", EndpointMode: "override", Endpoint: endpoint}})
	if r.Error != "" || r.Review == nil {
		t.Fatal("review failed:", r.Error)
	}
	return *r.Review
}
func providerSave(b *nativeProviderBackend, r modelcatalog.NativeProviderReview, key string) modelcatalog.NativeProviderResult {
	return b.Apply(modelcatalog.NativeProviderAction{RequestID: "save", Action: "save", Provider: r.Provider, ReviewID: r.ID, APIKey: key})
}
func requireNoProviderSecret(t *testing.T, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(nativeFixtureSecret)) {
		t.Fatal("fixture secret leaked into non-secret output")
	}
}
func TestNativeProviderSaveRetainReplaceRemoveAndNoLiveRequest(t *testing.T) {
	b := nativeProviderFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	credentials, err := auth.New(b.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{nativeFixtureSecret, "", "   ", nativeFixtureSecret + "_REPLACED"} {
		review := providerReview(t, b, strings.TrimSpace(key) != "", server.URL+"/v1")
		requireNoProviderSecret(t, review)
		r := providerSave(b, review, key)
		if !r.Confirmed || r.Error != "" {
			t.Fatal("save unconfirmed:", r.Error)
		}
		requireNoProviderSecret(t, r)
		actual, found, err := credentials.Read("openai")
		if err != nil || !found || !strings.HasPrefix(actual.Key, nativeFixtureSecret) {
			t.Fatal("credential not persisted")
		}
		settings, err := b.settings.Load()
		if err != nil || settings.Defaults.Provider != "openai" || settings.Defaults.Model != "fixture-model" || settings.ProviderEndpoint("openai") != server.URL+"/v1" {
			t.Fatal("settings not confirmed")
		}
		data, err := os.ReadFile(b.settings.Path())
		if err != nil || bytes.Contains(data, []byte(nativeFixtureSecret)) {
			t.Fatal("secret in settings")
		}
		if again := providerSave(b, review, key); again.Confirmed || again.Error == "" {
			t.Fatal("review reused")
		}
	}
	for _, path := range []string{credentials.Path(), b.settings.Path()} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("private permissions not preserved")
		}
	}
	if err := credentials.Put("anthropic", auth.Credential{Type: "api_key", Key: "unrelated-inert"}); err != nil {
		t.Fatal(err)
	}
	before, _ := b.settings.Load()
	review := b.Apply(modelcatalog.NativeProviderAction{Action: "review_remove", Provider: "openai"})
	if review.Review == nil {
		t.Fatal(review.Error)
	}
	r := b.Apply(modelcatalog.NativeProviderAction{Action: "remove", Provider: "openai", ReviewID: review.Review.ID})
	if !r.Confirmed {
		t.Fatal(r.Error)
	}
	_, found, err := credentials.Read("openai")
	if err != nil || found {
		t.Fatal("removal not persisted")
	}
	_, found, err = credentials.Read("anthropic")
	if err != nil || !found {
		t.Fatal("unrelated credential removed")
	}
	after, _ := b.settings.Load()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("removal changed configuration")
	}
	if calls.Load() != 0 {
		t.Fatal("setup performed a network request")
	}
}

func TestNativeProviderRejectsUnsupportedExpiredMismatchedAndSecretProjection(t *testing.T) {
	b := nativeProviderFixture(t)
	for _, provider := range []string{"pi", "ollama", "OPENAI", "custom-provider", nativeFixtureSecret} {
		r := b.Apply(modelcatalog.NativeProviderAction{Action: "review", Provider: provider})
		if r.Error == "" || r.Confirmed {
			t.Fatal("unsupported provider accepted")
		}
		requireNoProviderSecret(t, r)
	}
	for _, endpoint := range []string{"https://u:" + nativeFixtureSecret + "@example.invalid", "https://example.invalid?api_key=" + nativeFixtureSecret, "file:///tmp/key"} {
		r := b.Apply(modelcatalog.NativeProviderAction{Action: "review", Provider: "openai", Input: &modelcatalog.NativeProviderInput{Model: "fixture", EndpointMode: "override", Endpoint: endpoint}})
		if r.Error == "" {
			t.Fatal("unsafe endpoint accepted")
		}
		requireNoProviderSecret(t, r)
	}
	r := b.Apply(modelcatalog.NativeProviderAction{Action: "review", Provider: "openai", APIKey: nativeFixtureSecret})
	if r.Error == "" {
		t.Fatal("secret accepted before save")
	}
	requireNoProviderSecret(t, r)
	for _, mutation := range []string{"expired", "provider", "intent", "cancel"} {
		review := providerReview(t, b, true, "https://example.invalid/v1")
		switch mutation {
		case "expired":
			b.review.expires = time.Now().Add(-time.Second)
		case "provider":
			review.Provider = "anthropic"
		case "intent":
		case "cancel":
			b.Apply(modelcatalog.NativeProviderAction{Action: "cancel"})
		}
		key := nativeFixtureSecret
		if mutation == "intent" {
			key = ""
		}
		r = providerSave(b, review, key)
		if r.Confirmed || r.Error == "" {
			t.Fatal("invalid review accepted")
		}
		requireNoProviderSecret(t, r)
	}
	credentials, _ := auth.New(b.stateDir)
	_ = credentials.Put("openai", auth.Credential{Type: "oauth", Access: nativeFixtureSecret, Expires: time.Now().Add(-time.Hour).UnixMilli()})
	r = b.Apply(modelcatalog.NativeProviderAction{Action: "status"})
	requireNoProviderSecret(t, r)
	for _, row := range r.Rows {
		if row.ID == "openai" && (!row.Stored || !row.Expired || row.Ready) {
			t.Fatal("unsupported expired credential claimed ready")
		}
	}
}

// The helper changes the same canonical stores in an independent OS process.
// Its only secret input is stdin; neither argv nor environment carries it.
func TestNativeProviderExternalMutationHelper(t *testing.T) {
	if os.Getenv("GATOR_P04_STORE_HELPER") != "1" {
		return
	}
	var in struct{ Config, State, Key, Operation string }
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		os.Exit(2)
	}
	if in.Operation == "credential" {
		s, err := auth.New(in.State)
		if err != nil {
			os.Exit(3)
		}
		if s.Put("openai", auth.Credential{Type: "api_key", Key: in.Key}) != nil {
			os.Exit(4)
		}
	} else {
		s, err := config.New(in.Config)
		if err != nil {
			os.Exit(5)
		}
		if s.Update(func(v *config.Settings) error { v.Defaults.Model = "concurrent-terminal-model"; return nil }) != nil {
			os.Exit(6)
		}
	}
	os.Exit(0)
}
func externalProviderMutation(t *testing.T, b *nativeProviderBackend, kind string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestNativeProviderExternalMutationHelper$")
	cmd.Env = append(os.Environ(), "GATOR_P04_STORE_HELPER=1")
	input, _ := json.Marshal(map[string]string{"Config": filepath.Dir(filepath.Dir(b.settings.Path())), "State": b.stateDir, "Key": nativeFixtureSecret + "_OTHER_PROCESS", "Operation": kind})
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal("external mutation failed:", err)
	}
	if len(output) > 0 {
		t.Fatal("store helper unexpectedly produced output")
	}
}
func TestNativeProviderCrossProcessSettingsAndCredentialConflicts(t *testing.T) {
	for _, kind := range []string{"credential", "settings", "remove"} {
		t.Run(kind, func(t *testing.T) {
			b := nativeProviderFixture(t)
			credentials, _ := auth.New(b.stateDir)
			_ = credentials.Put("openai", auth.Credential{Type: "api_key", Key: "previous-inert"})
			review := providerReview(t, b, true, "https://example.invalid/v1")
			if kind == "remove" {
				r := b.Apply(modelcatalog.NativeProviderAction{Action: "review_remove", Provider: "openai"})
				review = *r.Review
			}
			mutation := kind
			if kind == "remove" {
				mutation = "credential"
			}
			externalProviderMutation(t, b, mutation)
			var r modelcatalog.NativeProviderResult
			if kind == "remove" {
				r = b.Apply(modelcatalog.NativeProviderAction{Action: "remove", Provider: "openai", ReviewID: review.ID})
			} else {
				r = providerSave(b, review, nativeFixtureSecret)
			}
			if !r.Conflict || r.Confirmed {
				t.Fatal("stale review did not conflict:", r.Error)
			}
			requireNoProviderSecret(t, r)
			current, found, err := credentials.Read("openai")
			if err != nil || !found {
				t.Fatal("credential lost")
			}
			expected := nativeFixtureSecret + "_OTHER_PROCESS"
			if kind == "settings" {
				expected = "previous-inert"
			}
			if current.Key != expected {
				t.Fatal("concurrent credential overwritten or rollback lost prior credential")
			}
			if kind == "settings" {
				s, _ := b.settings.Load()
				if s.Defaults.Model != "concurrent-terminal-model" {
					t.Fatal("concurrent settings overwritten")
				}
			}
		})
	}
}
func TestNativeProviderCompetingReviewsAndStorageFailure(t *testing.T) {
	b := nativeProviderFixture(t)
	other := &nativeProviderBackend{settings: b.settings, stateDir: b.stateDir}
	a := providerReview(t, b, true, "https://example.invalid/v1")
	c := providerReview(t, other, true, "https://example.invalid/v1")
	if !providerSave(b, a, nativeFixtureSecret).Confirmed {
		t.Fatal("first save failed")
	}
	r := providerSave(other, c, nativeFixtureSecret+"_SECOND")
	if !r.Conflict || r.Confirmed {
		t.Fatal("competing review overwrote first")
	}
	review := providerReview(t, b, true, "https://example.invalid/v1")
	credentials, _ := auth.New(b.stateDir)
	if err := os.Remove(credentials.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(credentials.Path(), 0700); err != nil {
		t.Fatal(err)
	}
	r = providerSave(b, review, nativeFixtureSecret)
	if r.Confirmed || r.Error == "" {
		t.Fatal("storage failure claimed success")
	}
	requireNoProviderSecret(t, r)
}
func TestNativeProviderAllowlistMatchesCanonicalDirectProviders(t *testing.T) {
	for _, name := range model.Names() {
		p, _ := model.ParseProvider(name)
		_, ok := supportedNativeProvider(name)
		if ok != model.SupportsAPIKeyLogin(p) {
			t.Fatal("allowlist disagrees")
		}
	}
}

func TestNativeProviderRedactsKnownStoredAndAmbientSecretsAcrossPublicFields(t *testing.T) {
	b := nativeProviderFixture(t)
	credentials, _ := auth.New(b.stateDir)
	if err := credentials.Put("anthropic", auth.Credential{Type: "api_key", Key: nativeFixtureSecret}); err != nil {
		t.Fatal(err)
	}
	if err := b.settings.Update(func(s *config.Settings) error {
		s.Defaults.Provider = "openai"
		s.Defaults.Model = nativeFixtureSecret
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r := b.Apply(modelcatalog.NativeProviderAction{Action: "status"})
	requireNoProviderSecret(t, r)
	t.Setenv("OPENAI_API_KEY", nativeFixtureSecret)
	r = b.Apply(modelcatalog.NativeProviderAction{Action: "review", Provider: "openai", Input: &modelcatalog.NativeProviderInput{Model: nativeFixtureSecret, EndpointMode: "default"}})
	if r.Error == "" {
		t.Fatal("known secret accepted in configuration")
	}
	requireNoProviderSecret(t, r)
}
