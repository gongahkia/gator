package edit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
)

func editSystemPrompt() string {
	return strings.Join([]string{
		"Emit only a unified diff.",
		"Use relative paths prefixed with a/ and b/.",
		"Use /dev/null for file creation or deletion.",
		"Do not wrap the diff in code fences.",
		"Do not include prose before or after the diff.",
	}, "\n")
}

func editPrompt(env *envelope.Envelope, useRawContext bool) string {
	contextLabel := "Context digest:"
	contextValue := fmt.Sprintf("%#v", env.Digest)
	if useRawContext {
		raw, _ := json.Marshal(env.Raw)
		contextLabel = "Raw context JSON:"
		contextValue = string(raw)
	}
	return strings.Join([]string{
		"Instruction:",
		env.Instruction,
		"",
		"Current step:",
		env.Plan.NextAction.Description,
		"",
		"Target path:",
		env.Plan.NextAction.TargetPath,
		"",
		contextLabel,
		contextValue,
	}, "\n")
}

func retryPrompt(env *envelope.Envelope, applyErr error, useRawContext bool) string {
	target := env.Plan.NextAction.TargetPath
	return strings.Join([]string{
		editPrompt(env, useRawContext),
		"",
		"Previous patch failed:",
		applyErr.Error(),
		"",
		"Current target file:",
		readTarget(env.Cwd, target),
	}, "\n")
}

func readTarget(cwd, rel string) string {
	path := filepath.Join(cwd, rel)
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(b) > 8192 {
		b = b[:8192]
	}
	return string(b)
}
