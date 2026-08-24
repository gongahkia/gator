package dependency

import (
	"errors"
	"testing"
)

func TestDetectWithLookupReportsApplicableMissingAndInstalledRequirements(t *testing.T) {
	statuses := DetectWithLookup("linux", func(executable string) (string, error) {
		if executable == "git" {
			return "/usr/bin/git", nil
		}
		return "", errors.New("not found")
	})
	if len(statuses) != 3 {
		t.Fatalf("dependency statuses = %#v", statuses)
	}
	if !statuses[0].Installed || statuses[0].ID != "git" || statuses[1].Installed || statuses[1].ID != "bubblewrap" || statuses[2].Installed || statuses[2].HelpURL == "" {
		t.Fatalf("dependency statuses = %#v", statuses)
	}
}

func TestDetectWithLookupExcludesLinuxOnlyDependencyElsewhere(t *testing.T) {
	statuses := DetectWithLookup("darwin", func(string) (string, error) { return "/installed", nil })
	foundSandboxExec := false
	for _, status := range statuses {
		if status.ID == "bubblewrap" {
			t.Fatalf("Darwin dependencies include Bubblewrap: %#v", statuses)
		}
		if status.ID == "sandbox-exec" {
			foundSandboxExec = true
		}
	}
	if !foundSandboxExec {
		t.Fatalf("Darwin dependencies omit sandbox-exec: %#v", statuses)
	}
}
