package security_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/egress"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/policy"
	"github.com/gongahkia/paw/internal/workspace"
)

func TestSecurityRegressionFixtures(t *testing.T) {
	root := t.TempDir()
	pathBypass := fixture(t, "path-traversal.txt")
	if _, err := workspace.ResolvePath(root, pathBypass); !errors.Is(err, workspace.ErrPathEscapesRoot) {
		t.Fatalf("path bypass error = %v", err)
	}

	secret := fixture(t, "secret-context.txt")
	result, manifest := egress.RedactForEgress(&envelope.RawContext{Units: []envelope.RawUnit{{ID: "u001", Text: secret}}})
	if strings.Contains(result.Raw.Units[0].Text, secret) || len(manifest.Findings) != 1 || manifest.Findings[0].Kind != "openai_key" {
		t.Fatalf("secret fixture result=%#v manifest=%#v", result, manifest)
	}

	cfg := config.Defaults().Policy
	cfg.Provider.AllowedTransports = append(cfg.Provider.AllowedTransports, "openai")
	cfg.Provider.AllowedBaseURLs = []string{"https://api.allowed.example/v1"}
	endpointBypass := fixture(t, "unallowlisted-endpoint.txt")
	if err := policy.CheckEndpoint(cfg, "openai", endpointBypass); !errors.Is(err, policy.ErrDenied) {
		t.Fatalf("endpoint bypass error = %v", err)
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}
