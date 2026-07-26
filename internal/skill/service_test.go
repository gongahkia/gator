package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectRejectsExecutableAndRequiresManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# test"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skill.json"), []byte(`{"id":"test","version":"1.0.0","name":"Test","tools":[{"id":"read","kind":"http"}]}`), 0o640); err != nil {
		t.Fatal(err)
	}
	pkg, findings, err := inspect(root)
	if err != nil || pkg.Digest == "" || findings["status"] != "pass" {
		t.Fatalf("pkg=%#v findings=%#v err=%v", pkg, findings, err)
	}
	if err := os.WriteFile(filepath.Join(root, "run.sh"), []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspect(root); err == nil {
		t.Fatal("executable bundle accepted")
	}
}

func TestDiscoverAndInspectNativeAndAdaptedSkills(t *testing.T) {
	root := t.TempDir()
	native := filepath.Join(root, "native", "calendar")
	adapted := filepath.Join(root, "external", "research")
	for _, path := range []string{native, adapted} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(native, "SKILL.md"), []byte("# calendar"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(native, "skill.json"), []byte(`{"id":"calendar","version":"1.0.0","name":"Calendar"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adapted, "skill.md"), []byte("---\nname: Research helper\ndescription: Find and cite sources\n---\n# Research"), 0o640); err != nil {
		t.Fatal(err)
	}
	candidates, err := discoverSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].Path != "external/research" || candidates[1].Path != "native/calendar" {
		t.Fatalf("candidates=%#v", candidates)
	}
	adaptedPackage, adaptedFindings, mode, err := inspectCandidate(candidates[0], ImportInput{SourceURI: "https://github.com/example/repo", SourceRef: "v1", Mode: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if mode != "adapted" || adaptedPackage.Name != "Research helper" || adaptedPackage.Version != "v1" || adaptedFindings["path"] != "external/research" {
		t.Fatalf("package=%#v findings=%#v mode=%s", adaptedPackage, adaptedFindings, mode)
	}
	if adaptedPackage.Manifest["mode"] != "adapted" || len(adaptedPackage.Manifest["tools"].([]any)) != 0 {
		t.Fatalf("adapted manifest=%#v", adaptedPackage.Manifest)
	}
	nativePackage, nativeFindings, mode, err := inspectCandidate(candidates[1], ImportInput{Mode: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if mode != "native" || nativePackage.ID != "calendar" || nativeFindings["mode"] != "native" {
		t.Fatalf("package=%#v findings=%#v mode=%s", nativePackage, nativeFindings, mode)
	}
}

func TestAdaptedSkillRejectsExecutableFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# external"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "install.sh"), []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := inspectCandidate(discoveredSkill{Root: root, Path: "."}, ImportInput{SourceURI: "https://github.com/example/repo", Mode: "adapted"})
	if err == nil {
		t.Fatal("adapted skill accepted executable")
	}
}
