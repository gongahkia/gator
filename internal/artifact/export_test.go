package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWriteArchiveExportsOnlyPortableVerifiedBundle(t *testing.T) {
	bundlePath, _ := sealedBundleFixture(t)
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var first bytes.Buffer
	if err := WriteArchive(&first, bundle); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := WriteArchive(&second, bundle); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("identical bundle produced nondeterministic archives")
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	tarReader := tar.NewReader(gzipReader)
	var names []string
	var report []byte
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
		if header.Name == "output/report.md" {
			report, err = io.ReadAll(tarReader)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if want := []string{"manifest.json", "output/", "output/report.md"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("archive entries = %#v, want %#v", names, want)
	}
	if string(report) != "sealed report\n" {
		t.Fatalf("archived report = %q", report)
	}
}

func TestWriteArchiveRefusesModifiedBundleBeforeWriting(t *testing.T) {
	bundlePath, _ := sealedBundleFixture(t)
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundlePath, "output", "report.md"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteArchive(&output, bundle); err == nil {
		t.Fatal("tampered bundle was exported")
	}
	if output.Len() != 0 {
		t.Fatalf("failed export wrote %d bytes", output.Len())
	}
}
