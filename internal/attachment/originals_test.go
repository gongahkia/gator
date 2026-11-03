package attachment

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/document"
)

func TestUploadedOriginalAndProviderViewComeFromSameBytes(t *testing.T) {
	payload, _, err := document.RenderDOCX(document.Spec{Title: "Selected report", Blocks: []document.Block{{Kind: "paragraph", Text: "Human-readable report content"}}})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	id := strings.Repeat("a", 32) + "/report.docx"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(directory, id)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, id), payload, 0600); err != nil {
		t.Fatal(err)
	}
	_, documents, files, err := LoadUploadsWithOriginals(directory, []string{id})
	if err != nil || len(files) != 1 || len(documents) != 1 {
		t.Fatal("load original", err)
	}
	if documents[0].MediaType != "text/plain" || !bytes.Contains(documents[0].Data, []byte("Human-readable report content")) || bytes.Equal(documents[0].Data, payload) || !bytes.Equal(files[0].Data, payload) || files[0].Name != "report.docx" {
		t.Fatal("original and extracted representation confused")
	}
	if err := os.WriteFile(filepath.Join(directory, id), []byte("changed after loading"), 0600); err != nil {
		t.Fatal(err)
	}
	documents[0].Data[0] = 'X'
	if !bytes.Equal(files[0].Data, payload) {
		t.Fatal("provider view or disk mutation changed loaded original")
	}
	selected, err := SelectOriginals(files)
	if err != nil {
		t.Fatal(err)
	}
	metadata := selected.Files()
	if len(metadata) != 1 || metadata[0].Source != "input/1/report.docx" || metadata[0].Bytes != int64(len(payload)) || len(metadata[0].SHA256) != 64 {
		t.Fatal("missing exact original metadata", metadata)
	}
	files[0].Data[0] = 'X'
	data, err := selected.Read(metadata[0].Source, int64(MaxDocumentBytes))
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatal("selection did not freeze original", err)
	}
	data[0] = 'Y'
	metadata[0].Filename = "mutated.docx"
	data, err = selected.Read("input/1/report.docx", int64(MaxDocumentBytes))
	if err != nil || !bytes.Equal(data, payload) || selected.Files()[0].Filename != "report.docx" {
		t.Fatal("reader changed the selection")
	}
	for _, reference := range []string{id, "input/2/report.docx", "input/1/other.docx", "input/1/../report.docx", "/private/report.docx"} {
		if _, err := selected.Read(reference, int64(MaxDocumentBytes)); err == nil {
			t.Fatal("read an unselected reference", reference)
		}
	}
	if _, err := selected.Read("input/1/report.docx", 1); err == nil {
		t.Fatal("read limit bypassed")
	}
	encoded, _ := json.Marshal(files)
	if bytes.Contains(encoded, []byte(base64.StdEncoding.EncodeToString(payload))) || bytes.Contains(encoded, []byte("Human-readable report content")) {
		t.Fatal("original bytes serialized")
	}
}

func TestOriginalSelectionLimitsAndDuplicateNames(t *testing.T) {
	for _, file := range []Original{{Name: "../x.txt", Data: []byte("x")}, {Name: "x\ny.txt", Data: []byte("x")}, {Name: "x.txt"}, {Name: "x.png", Data: make([]byte, MaxImageBytes+1)}} {
		if _, err := SelectOriginals([]Original{file}); err == nil {
			t.Fatal("invalid original accepted", file.Name)
		}
	}
	large := make([]byte, MaxDocumentBytes)
	if err := ValidateOriginals([]Original{{Name: "a.docx", Data: large}, {Name: "b.docx", Data: large}}); err != nil {
		t.Fatal("exact aggregate limit rejected", err)
	}
	if err := ValidateOriginals([]Original{{Name: "a.docx", Data: large}, {Name: "b.docx", Data: large}, {Name: "c.txt", Data: []byte("x")}}); err == nil {
		t.Fatal("aggregate original limit bypassed")
	}
	selection, err := SelectOriginals([]Original{{Name: "same.txt", Data: []byte("one")}, {Name: "same.txt", Data: []byte("two")}})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := selection.Read("input/1/same.txt", 10)
	b, _ := selection.Read("input/2/same.txt", 10)
	if string(a) != "one" || string(b) != "two" {
		t.Fatal("duplicate filenames alias the same selected file")
	}
}
