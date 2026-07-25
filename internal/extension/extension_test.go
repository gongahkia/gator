package extension

import (
	"context"
	"testing"

	"github.com/gongahkia/norbot/internal/domain"
)

type testProvider struct{ id, version string }

func (p testProvider) ID() string                                        { return p.id }
func (p testProvider) APIVersion() string                                { return p.version }
func (p testProvider) Supports(domain.Stage) bool                        { return true }
func (p testProvider) Invoke(context.Context, Request) (Response, error) { return Response{}, nil }

func TestRegistryRejectsIncompatibleVersion(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterProvider(testProvider{id: "provider", version: "v0"}); err == nil {
		t.Fatal("expected version error")
	}
	if err := registry.RegisterProvider(testProvider{id: "provider", version: APIVersion}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(testProvider{id: "provider", version: APIVersion}); err == nil {
		t.Fatal("expected duplicate error")
	}
}
