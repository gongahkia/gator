package edit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
	patcher "github.com/gongahkia/paw/internal/patch"
)

type Edit struct {
	Client llm.Client
}

type ApplyFailure struct {
	Err error
}

func (e *ApplyFailure) Error() string {
	return fmt.Sprintf("edit apply failed: %v", e.Err)
}

func (e *ApplyFailure) Unwrap() error {
	return e.Err
}

func New(client llm.Client) *Edit {
	return &Edit{Client: client}
}

func (e *Edit) Name() string {
	return "edit"
}

func (e *Edit) Run(ctx context.Context, in *envelope.Envelope) (*envelope.Envelope, error) {
	out := *in
	out.Stage = e.Name()
	if in.Plan == nil || in.Plan.NextAction == nil || in.Plan.NextAction.Kind != "edit_file" {
		return &out, nil
	}
	if e.Client == nil {
		return nil, fmt.Errorf("edit client is nil")
	}
	diff, err := e.askAndApply(ctx, in, editPrompt(in))
	if err == nil {
		out.Patch = &envelope.Patch{UnifiedDiff: diff, Files: diffFiles(diff)}
		return &out, nil
	}
	retryDiff, retryErr := e.askAndApply(ctx, in, retryPrompt(in, err))
	if retryErr != nil {
		return nil, &ApplyFailure{Err: retryErr}
	}
	out.Patch = &envelope.Patch{UnifiedDiff: retryDiff, Files: diffFiles(retryDiff)}
	return &out, nil
}

func (e *Edit) askAndApply(ctx context.Context, env *envelope.Envelope, prompt string) (string, error) {
	resp, err := e.Client.Chat(ctx, llm.ChatRequest{
		Messages: []llm.ChatMessage{
			{Role: "system", Content: "Return only a unified diff. No prose, no fences."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", err
	}
	diff, err := patcher.Extract(resp.Content)
	if err != nil {
		return "", err
	}
	if err := patcher.Apply(env.Cwd, diff); err != nil {
		return "", err
	}
	return diff, nil
}

func editPrompt(env *envelope.Envelope) string {
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
		"Context digest:",
		fmt.Sprintf("%#v", env.Digest),
	}, "\n")
}

func retryPrompt(env *envelope.Envelope, applyErr error) string {
	target := env.Plan.NextAction.TargetPath
	return strings.Join([]string{
		editPrompt(env),
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

func diffFiles(diff string) []string {
	seen := map[string]bool{}
	var files []string
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+++ ") || strings.Contains(line, "/dev/null") {
			continue
		}
		path := strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")), "b/")
		if path != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	return files
}
