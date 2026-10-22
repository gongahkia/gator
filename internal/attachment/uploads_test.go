package attachment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadsStayWithinPrivateRootAndUseExistingValidation(t *testing.T) {
	root := t.TempDir()
	id := strings.Repeat("a", 32) + "/brief.txt"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, id)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, id), []byte("Only the chosen brief"), 0600); err != nil {
		t.Fatal(err)
	}
	_, documents, err := LoadUploads(root, []string{id})
	if err != nil || len(documents) != 1 || documents[0].Name != "brief.txt" || string(documents[0].Data) != "Only the chosen brief" {
		t.Fatalf("documents = %#v, %v", documents, err)
	}
	for _, invalid := range []string{"../brief.txt", filepath.Join(root, id), strings.Repeat("a", 32) + "/..", strings.Repeat("a", 32) + "/x\\y.txt"} {
		if _, _, err := LoadUploads(root, []string{invalid}); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	if _, _, err := LoadUploads(root, []string{id, id}); err == nil {
		t.Fatal("accepted duplicate upload")
	}
	bad := strings.Repeat("a", 32) + "/fake.png"
	if err := os.WriteFile(filepath.Join(root, bad), []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadUploads(root, []string{bad}); err == nil {
		t.Fatal("accepted forged image bytes")
	}
	if _, _, err := LoadUploads("", []string{id}); err == nil {
		t.Fatal("accepted missing host root")
	}
}
