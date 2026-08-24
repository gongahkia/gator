package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

func TestLoadPromptAttachmentsRequiresRepositoryLocalSupportedFiles(t *testing.T) {
	repository := t.TempDir()
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	if err := os.WriteFile(filepath.Join(repository, "screen.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "notes.md"), []byte("reference"), 0o644); err != nil {
		t.Fatal(err)
	}
	images, attachments, err := loadPromptAttachments(repository, []string{"screen.png"}, []string{"notes.md"})
	if err != nil {
		t.Fatalf("load prompt attachments: %v", err)
	}
	if len(images) != 1 || images[0].Name != "screen.png" || len(attachments) != 1 || attachments[0].Name != "notes.md" {
		t.Fatalf("inputs = images %#v, attachments %#v", images, attachments)
	}
	if _, _, err := loadPromptAttachments(repository, []string{"../outside.png"}, nil); err == nil || !strings.Contains(err.Error(), "escapes the workspace") {
		t.Fatalf("outside attachment error = %v", err)
	}
}

func TestAttachmentFlagsAndSummary(t *testing.T) {
	var paths attachmentFlags
	if err := paths.Set("screen.png"); err != nil {
		t.Fatalf("set attachment path: %v", err)
	}
	if err := paths.Set(" "); err == nil {
		t.Fatal("empty attachment path was accepted")
	}
	images, attachments, err := loadPromptAttachments(t.TempDir(), nil, nil)
	if err != nil || len(images) != 0 || len(attachments) != 0 {
		t.Fatalf("empty inputs = images %#v, attachments %#v, error %v", images, attachments, err)
	}
	var output bytes.Buffer
	if err := writePromptAttachmentSummary(&output, []agent.Image{{Name: "screen.png", MediaType: "image/png", Data: []byte("pixels")}}, []agent.Attachment{{Name: "notes.md", MediaType: "text/plain", Data: []byte("notes")}}); err != nil {
		t.Fatalf("write attachment summary: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "screen.png (image/png, 6 bytes)") || !strings.Contains(got, "notes.md (text/plain, 5 bytes)") {
		t.Fatalf("attachment summary = %q", got)
	}
}

func TestRunTaskRejectsEscapingExplicitImageBeforeStartingProvider(t *testing.T) {
	repository := t.TempDir()
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Chdir(repository)

	err := runTask([]string{
		"--provider", "cursor",
		"--verify", "true",
		"--image", "../outside.png",
		"describe the screenshot",
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "escapes the workspace") {
		t.Fatalf("run attachment error = %v", err)
	}
}
