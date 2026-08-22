package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/mcp"
)

func TestMCPLogoutRemovesOnlyTheConfiguredRemoteCredential(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	repository := t.TempDir()
	command := exec.Command("git", "init", "--quiet")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initialize repository: %v: %s", err, output)
	}
	if err := os.MkdirAll(filepath.Join(repository, ".gator"), 0o755); err != nil {
		t.Fatal(err)
	}
	endpoint := "https://mcp.example.test/mcp"
	manifest := `{"version":1,"servers":[{"name":"remote","transport":"streamable_http","url":"` + endpoint + `"}]}`
	if err := os.WriteFile(filepath.Join(repository, ".gator", "mcp.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	key, err := mcp.OAuthCredentialKey(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	credential := auth.Credential{Type: "oauth", Access: "bound-token", Expires: time.Now().Add(time.Hour).UnixMilli(), Extra: map[string]string{"mcp_resource": endpoint, "mcp_token_url": "https://mcp.example.test/token", "mcp_client_id": "client"}}
	if err := credentials.Put(key, credential); err != nil {
		t.Fatal(err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repository); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	var output bytes.Buffer
	if err := mcpCommand([]string{"status"}, &output); err != nil {
		t.Fatalf("MCP status: %v", err)
	}
	if !strings.Contains(output.String(), "OAuth remote: authenticated") {
		t.Fatalf("status output = %q", output.String())
	}
	output.Reset()
	if err := mcpCommand([]string{"logout", "remote"}, &output); err != nil {
		t.Fatalf("logout MCP server: %v", err)
	}
	if !strings.Contains(output.String(), "Removed the Gator OAuth credential") {
		t.Fatalf("logout output = %q", output.String())
	}
	if _, found, err := credentials.Read(key); err != nil || found {
		t.Fatalf("credential after logout: found=%t err=%v", found, err)
	}
}
