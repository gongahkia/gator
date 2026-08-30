package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseUpdaterInstallsChecksummedArchive(t *testing.T) {
	archive := tarArchive(t, "gator_v1.2.3_darwin_arm64/gator", []byte("new executable"))
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			_, _ = writer.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"gator_darwin_arm64.tar.gz","browser_download_url":"` + serverURL(request) + `/archive"},{"name":"checksums.txt","browser_download_url":"` + serverURL(request) + `/checksums"}]}`))
		case "/archive":
			_, _ = writer.Write(archive)
		case "/checksums":
			_, _ = writer.Write([]byte(hex.EncodeToString(digest[:]) + "  gator_darwin_arm64.tar.gz\n"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "gator")
	if err := os.WriteFile(target, []byte("old executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	updater := releaseUpdater{Client: server.Client(), APIURL: server.URL + "/latest", GOOS: "darwin", GOARCH: "arm64", Version: "v1.0.0", Executable: func() (string, error) { return target, nil }}
	latest, err := updater.latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if err := updater.install(latest); err != nil {
		t.Fatalf("install: %v", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replacement: %v", err)
	}
	if string(contents) != "new executable" {
		t.Fatalf("replacement = %q", contents)
	}
}

func TestReleaseUpdaterRejectsChecksumMismatch(t *testing.T) {
	archive := tarArchive(t, "gator_v1.2.3_darwin_arm64/gator", []byte("new executable"))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/archive":
			_, _ = writer.Write(archive)
		case "/checksums":
			_, _ = writer.Write([]byte(strings.Repeat("0", 64) + "  gator_darwin_arm64.tar.gz\n"))
		}
	}))
	defer server.Close()
	updater := releaseUpdater{Client: server.Client(), GOOS: "darwin", GOARCH: "arm64", Executable: func() (string, error) { return filepath.Join(t.TempDir(), "gator"), nil }}
	err := updater.install(release{TagName: "v1.2.3", Assets: []releaseAsset{{Name: "gator_darwin_arm64.tar.gz", URL: server.URL + "/archive"}, {Name: "checksums.txt", URL: server.URL + "/checksums"}}})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("install error = %v", err)
	}
}

func TestReleaseUpdaterRejectsUnsupportedPlatform(t *testing.T) {
	updater := releaseUpdater{GOOS: "freebsd", GOARCH: "amd64"}
	if err := updater.install(release{}); err == nil || !strings.Contains(err.Error(), "only Linux and macOS") {
		t.Fatalf("unsupported update platform error = %v", err)
	}
}

func TestNewerVersion(t *testing.T) {
	for _, test := range []struct {
		current, candidate string
		want               bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.2.0", "v1.1.9", false},
		{"v1.2.0", "v1.2.0", false},
		{"dev", "v1.2.0", true},
	} {
		got, err := newerVersion(test.current, test.candidate)
		if err != nil || got != test.want {
			t.Fatalf("newerVersion(%q, %q) = %v, %v; want %v", test.current, test.candidate, got, err, test.want)
		}
	}
}

func tarArchive(t *testing.T, name string, contents []byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	writer := tar.NewWriter(compressed)
	if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func serverURL(request *http.Request) string {
	return "http://" + request.Host
}
