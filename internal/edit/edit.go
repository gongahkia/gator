package edit

import (
	"context"
	"fmt"
	"strings"

	"github.com/gongahkia/paw/internal/budget"
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
	diff, err := e.askAndApply(ctx, &out, editPrompt(&out))
	if err == nil {
		out.Patch = &envelope.Patch{UnifiedDiff: diff, Files: diffFiles(diff)}
		return &out, nil
	}
	retryDiff, retryErr := e.askAndApply(ctx, &out, retryPrompt(&out, err))
	if retryErr != nil {
		return nil, &ApplyFailure{Err: retryErr}
	}
	out.Patch = &envelope.Patch{UnifiedDiff: retryDiff, Files: diffFiles(retryDiff)}
	return &out, nil
}

func (e *Edit) askAndApply(ctx context.Context, env *envelope.Envelope, prompt string) (string, error) {
	resp, err := e.Client.Chat(ctx, llm.ChatRequest{
		Messages: []llm.ChatMessage{
			{Role: "system", Content: editSystemPrompt()},
			{Role: "user", Content: prompt},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", err
	}
	budget.AddBrain(&env.Budget, resp.Usage.InputTokens, resp.Usage.OutputTokens)
	diff, err := patcher.Extract(resp.Content)
	if err != nil {
		return "", err
	}
	if err := patcher.Apply(env.Cwd, diff); err != nil {
		return "", err
	}
	return diff, nil
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
