package connector

import (
	"reflect"
	"testing"

	"github.com/gongahkia/gator/internal/action"
)

func TestRegistryClassifiesStableHTTPJSONOperation(t *testing.T) {
	descriptor := Descriptor{
		Version: DescriptorVersion, ID: "sales-data", Name: "Sales data",
		Kind: KindHTTPJSON, Resource: "https://example.com/report.json?period=current", Authentication: AuthBearer,
	}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	got, operation, err := registry.Operation("sales-data", "fetch")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, descriptor) || operation.Capability != action.ConnectedRead || string(operation.InputSchema) == "" {
		t.Fatalf("descriptor=%#v operation=%#v", got, operation)
	}
	if descriptor.CredentialRef() == "" || descriptor.CredentialRef() == (Descriptor{ID: descriptor.ID, Resource: "https://other.example/data"}).CredentialRef() {
		t.Fatal("credential reference is not resource-bound")
	}
	oauth := descriptor
	oauth.Authentication = AuthOAuth
	oauth.OAuthClientID = "one"
	oauth.OAuthTokenURL = "https://auth.example/token"
	changed := oauth
	changed.OAuthClientID = "two"
	if oauth.CredentialRef() == changed.CredentialRef() {
		t.Fatal("OAuth app identity did not change credential reference")
	}
}

func TestRegistryClassifiesWebhookAsPublish(t *testing.T) {
	t.Parallel()
	descriptor := Descriptor{
		Version: DescriptorVersion, ID: "release-hook", Name: "Release hook",
		Kind: KindHTTPWebhook, Resource: "https://example.com/releases", Authentication: AuthBearer,
	}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	_, operation, err := registry.Operation(descriptor.ID, "publish")
	if err != nil {
		t.Fatal(err)
	}
	if operation.Capability != action.Publish || !action.RequiresFreshApproval(operation.Capability) {
		t.Fatalf("webhook operation = %#v", operation)
	}
}

func TestRegistrySortsAndRejectsUnsafeDescriptors(t *testing.T) {
	valid := func(id, resource string) Descriptor {
		return Descriptor{Version: DescriptorVersion, ID: id, Name: id, Kind: KindHTTPJSON, Resource: resource, Authentication: AuthNone}
	}
	registry, err := NewRegistry([]Descriptor{valid("z-data", "https://z.example/data"), valid("a-data", "https://a.example/data")})
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.List(); got[0].ID != "a-data" || got[1].ID != "z-data" {
		t.Fatalf("registry order = %#v", got)
	}
	for _, descriptor := range []Descriptor{
		valid("Bad_ID", "https://example.com/data"),
		valid("data", "http://example.com/data"),
		valid("data", "https://user:pass@example.com/data"),
		valid("data", "https://example.com/data#fragment"),
		{Version: DescriptorVersion, ID: "data", Name: "data", Kind: "shell", Resource: "https://example.com", Authentication: AuthNone},
	} {
		if _, err := NewRegistry([]Descriptor{descriptor}); err == nil {
			t.Fatalf("unsafe descriptor accepted: %#v", descriptor)
		}
	}
	if _, err := NewRegistry([]Descriptor{valid("data", "https://example.com/one"), valid("data", "https://example.com/two")}); err == nil {
		t.Fatal("duplicate connector ID was accepted")
	}
}
