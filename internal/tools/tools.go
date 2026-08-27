package tools

import (
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

// Default returns the minimal local tool surface for one run. Callers provide
// the command policy because command authorization is project-specific.
func Default(root workspace.Root, policy CommandPolicy, additionalRoots ...workspace.Root) []agent.Tool {
	return []agent.Tool{
		ReadFile{Root: root, AdditionalRoots: additionalRoots},
		ListFiles{Root: root, AdditionalRoots: additionalRoots},
		SearchFiles{Root: root, AdditionalRoots: additionalRoots},
		ApplyPatch{Root: root},
		RunCommand{Root: root, Policy: policy},
		GitStatus{Root: root},
		GitDiff{Root: root},
	}
}

// ReadOnly returns the native tool surface available in an enforced planning
// turn. It intentionally excludes patching and command execution.
func ReadOnly(root workspace.Root, additionalRoots ...workspace.Root) []agent.Tool {
	return []agent.Tool{
		ReadFile{Root: root, AdditionalRoots: additionalRoots},
		ListFiles{Root: root, AdditionalRoots: additionalRoots},
		SearchFiles{Root: root, AdditionalRoots: additionalRoots},
		GitStatus{Root: root},
		GitDiff{Root: root},
	}
}
