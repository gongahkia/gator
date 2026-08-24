// Package dependency describes host prerequisites without installing or
// executing anything. Callers can present its guidance after explicit user
// interaction while retaining control of package-manager authority.
package dependency

import (
	"os/exec"
	"runtime"
)

// Requirement is one executable prerequisite Gator can detect and explain.
// Required describes the feature named by Purpose, not every Gator command.
type Requirement struct {
	ID           string
	Name         string
	Executable   string
	Purpose      string
	Required     bool
	Applicable   func(string) bool
	HelpURL      string
	Instructions func(string) []string
}

// Status is a display-safe detection result. It contains neither environment
// values nor package-manager credentials.
type Status struct {
	Requirement
	Installed bool
	Path      string
}

// Catalog returns the checked-in, extensible prerequisite registry. Gator
// intentionally offers instructions only; it never runs a package manager or
// a remote installer on the developer's behalf.
func Catalog() []Requirement {
	return []Requirement{
		{
			ID:         "git",
			Name:       "Git",
			Executable: "git",
			Purpose:    "repository discovery, worktrees, and review",
			Required:   true,
			Applicable: allPlatforms,
			HelpURL:    "https://git-scm.com/downloads",
			Instructions: func(os string) []string {
				if os == "linux" {
					return []string{"Install your distribution's git package (for Fedora: sudo dnf install git)."}
				}
				return []string{"Install Git using the official download for your operating system."}
			},
		},
		{
			ID:         "bubblewrap",
			Name:       "Bubblewrap",
			Executable: "bwrap",
			Purpose:    "strict Linux sandboxing",
			Required:   true,
			Applicable: func(os string) bool { return os == "linux" },
			HelpURL:    "https://github.com/containers/bubblewrap",
			Instructions: func(string) []string {
				return []string{"Install the bubblewrap package (for Fedora: sudo dnf install bubblewrap).", "After installation, run 'gator doctor' again to verify strict sandbox availability."}
			},
		},
		{
			ID:         "sandbox-exec",
			Name:       "sandbox-exec",
			Executable: "sandbox-exec",
			Purpose:    "strict macOS sandboxing",
			Required:   true,
			Applicable: func(os string) bool { return os == "darwin" },
			HelpURL:    "https://developer.apple.com/documentation/security/app_sandbox",
			Instructions: func(string) []string {
				return []string{"sandbox-exec is supplied by macOS. Restore the matching Apple system component, then run 'gator doctor' again to verify strict sandbox availability."}
			},
		},
		{
			ID:         "ollama",
			Name:       "Ollama",
			Executable: "ollama",
			Purpose:    "reviewed local coding models",
			Required:   false,
			Applicable: allPlatforms,
			HelpURL:    "https://ollama.com/download",
			Instructions: func(string) []string {
				return []string{"Install Ollama from its official download page, then return to Gator and refresh the Local model section."}
			},
		},
	}
}

// Detect reports only applicable catalog entries using the host's executable
// path. The injectable lookup keeps detection deterministic in tests.
func Detect() []Status {
	return DetectWithLookup(runtime.GOOS, exec.LookPath)
}

func DetectWithLookup(operatingSystem string, lookup func(string) (string, error)) []Status {
	result := make([]Status, 0, len(Catalog()))
	for _, requirement := range Catalog() {
		if requirement.Applicable != nil && !requirement.Applicable(operatingSystem) {
			continue
		}
		path, err := lookup(requirement.Executable)
		result = append(result, Status{Requirement: requirement, Installed: err == nil, Path: path})
	}
	return result
}

func allPlatforms(string) bool { return true }
