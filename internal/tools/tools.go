package tools

import (
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

// Default returns the minimal local tool surface for one run. Callers provide
// the command policy because command authorization is project-specific.
func Default(root workspace.Root, policy CommandPolicy) []agent.Tool {
	return []agent.Tool{
		ReadFile{Root: root},
		ListFiles{Root: root},
		SearchFiles{Root: root},
		ApplyPatch{Root: root},
		RunCommand{Root: root, Policy: policy},
		GitStatus{Root: root},
		GitDiff{Root: root},
	}
}
