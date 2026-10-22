package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/privatefs"
)

func TestWindowsBrowserEndpointRejectsNonLoopback(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	if err := privatefs.MkdirAll(directory); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "endpoint")
	for _, value := range []string{"192.0.2.1:4321", "0.0.0.0:4321", "localhost:4321", "127.0.0.1:0", "127.0.0.1:99999", "file://example"} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		connection, err := dialController(context.Background(), path)
		if err == nil {
			connection.Close()
			t.Fatalf("accepted invalid endpoint %s", value)
		}
	}
}
