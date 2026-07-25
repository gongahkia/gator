package artifact

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gongahkia/norbot/internal/config"
)

func TestS3StoreTransfersAndSignsObjects(t *testing.T) {
	var stored []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bucket/norbot/channel-input/account/message/1-file.txt" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("S3 request was not signed")
		}
		switch r.Method {
		case http.MethodPut:
			stored, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(stored)
		case http.MethodDelete:
			stored = nil
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("method=%s", r.Method)
		}
	}))
	defer server.Close()
	t.Setenv("TEST_S3_ACCESS", "access")
	t.Setenv("TEST_S3_SECRET", "secret")
	store, err := New(config.ArtifactStore{Enabled: true, Endpoint: server.URL, Region: "us-east-1", Bucket: "bucket", AccessKeyEnv: "TEST_S3_ACCESS", SecretKeyEnv: "TEST_S3_SECRET", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	key, err := Key("channel-input", "account", "message", "1-file.txt")
	if err != nil {
		t.Fatal(err)
	}
	put, err := store.Put(context.Background(), Object{Key: key, ContentType: "text/plain"}, []byte("hello"))
	if err != nil || put.Digest == "" || put.Size != 5 {
		t.Fatalf("put=%#v err=%v", put, err)
	}
	reader, object, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || string(data) != "hello" || object.ContentType != "text/plain" {
		t.Fatalf("data=%q object=%#v read=%v close=%v", data, object, readErr, closeErr)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactConfigurationAndKeysFailClosed(t *testing.T) {
	if _, err := New(config.ArtifactStore{Enabled: true, Endpoint: "https://s3.example.test", Bucket: "bucket", AccessKeyEnv: "MISSING_ACCESS", SecretKeyEnv: "MISSING_SECRET"}); err == nil {
		t.Fatal("missing credentials accepted")
	}
	if _, err := Key("channel-input", "../escape"); err == nil {
		t.Fatal("unsafe object key accepted")
	}
	if _, err := New(config.ArtifactStore{}); err != nil {
		t.Fatal(err)
	}
}
