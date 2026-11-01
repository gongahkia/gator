package speech

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type localTransport func(*http.Request) (*http.Response, error)

func (f localTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func localFixture(t *testing.T) (*LocalStore, []byte) {
	t.Helper()
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("a reviewed fake model used only by deterministic tests")
	sum := sha256.Sum256(data)
	store.catalog = []LocalModel{{ID: "tiny", Label: "Test only", Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}}
	return store, data
}
func modelResponse(data []byte) *http.Response {
	return &http.Response{StatusCode: 200, ContentLength: int64(len(data)), Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}
}
func TestLocalModelDownloadIsPinnedVerifiedAndPrivate(t *testing.T) {
	store, data := localFixture(t)
	calls := 0
	store.client = &http.Client{Transport: localTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://huggingface.co/ggerganov/whisper.cpp/resolve/"+whisperRevision+"/ggml-tiny.bin" || r.Header.Get("Authorization") != "" {
			t.Fatal("caller changed the model download authority")
		}
		return modelResponse(data), nil
	})}
	if err := store.Download(context.Background(), "tiny"); err != nil {
		t.Fatal(err)
	}
	path, err := store.VerifiedModel(context.Background(), "tiny")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != store.directory || calls != 1 || !store.Models()[0].Available {
		t.Fatal("model publication failed")
	}
	if err := store.Download(context.Background(), "../../secret"); err == nil || calls != 1 {
		t.Fatal("arbitrary model path reached the network")
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", len(data))), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.VerifiedModel(context.Background(), "tiny"); err == nil {
		t.Fatal("corrupt weights accepted")
	}
}
func TestLocalModelFailedDownloadsPreserveExistingModelAndRemovePartialFiles(t *testing.T) {
	for _, kind := range []string{"checksum", "oversized", "truncated", "body-error"} {
		t.Run(kind, func(t *testing.T) {
			store, data := localFixture(t)
			original := filepath.Join(store.directory, "ggml-tiny.bin")
			if err := os.WriteFile(original, data, 0600); err != nil {
				t.Fatal(err)
			}
			store.client = &http.Client{Transport: localTransport(func(r *http.Request) (*http.Response, error) {
				switch kind {
				case "checksum":
					return modelResponse([]byte(strings.Repeat("x", len(data)))), nil
				case "oversized":
					return modelResponse(append(data, 'x')), nil
				case "truncated":
					return modelResponse(data[:3]), nil
				default:
					return nil, errors.New("private provider diagnostic/token must not leak")
				}
			})}
			err := store.Download(context.Background(), "tiny")
			if err == nil || strings.Contains(err.Error(), "private provider") {
				t.Fatalf("unsafe failure: %v", err)
			}
			got, _ := os.ReadFile(original)
			if string(got) != string(data) {
				t.Fatal("failed download replaced previous weights")
			}
			entries, _ := os.ReadDir(store.directory)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".download-") {
					t.Fatal("partial model was retained")
				}
			}
		})
	}
}
func TestLocalModelDownloadCancellationAndConcurrentAdmission(t *testing.T) {
	store, _ := localFixture(t)
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	store.client = &http.Client{Transport: localTransport(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	go func() { done <- store.Download(ctx, "tiny") }()
	<-started
	if err := store.Download(context.Background(), "tiny"); err == nil {
		t.Fatal("concurrent download was admitted")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled download succeeded")
	}
}
func TestLocalModelNeverFollowsLinkedStorageOrDestinations(t *testing.T) {
	store, data := localFixture(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(store.directory, "ggml-tiny.bin")); err != nil {
		t.Skip(err)
	}
	if store.Models()[0].Available {
		t.Fatal("linked model was offered as downloaded")
	}
	if _, err := store.VerifiedModel(context.Background(), "tiny"); err == nil {
		t.Fatal("linked model was verified")
	}
	if err := store.Download(context.Background(), "tiny"); err == nil {
		t.Fatal("linked destination was replaced")
	}
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "gator")); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLocalStore(root); err == nil {
		t.Fatal("linked catalog parent accepted")
	}
}

func TestLocalModelRejectsUntrustedRedirectAndCleansOnlyItsAbandonedChunks(t *testing.T) {
	store, data := localFixture(t)
	abandoned := ".download-0123456789abcdef01234567"
	unrelated := ".download-user-notes"
	if err := os.WriteFile(filepath.Join(store.directory, abandoned), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.directory, unrelated), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	store.client = &http.Client{Transport: localTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://untrusted.example/private"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	if err := store.Download(context.Background(), "tiny"); err == nil || calls != 1 {
		t.Fatalf("untrusted redirect executed: calls=%d error=%v", calls, err)
	}
	if _, err := os.Stat(filepath.Join(store.directory, abandoned)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("abandoned Gator chunk retained")
	}
	if got, _ := os.ReadFile(filepath.Join(store.directory, unrelated)); string(got) != "keep" {
		t.Fatal("unrelated file removed")
	}
	store.client = &http.Client{Transport: localTransport(func(r *http.Request) (*http.Response, error) { return modelResponse(data), nil })}
	if err := store.Download(context.Background(), "tiny"); err != nil {
		t.Fatal(err)
	}
}
