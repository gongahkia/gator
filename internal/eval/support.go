package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gongahkia/gator/internal/agent"
)

var identifierPattern = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`)

// ScriptedModel is the deterministic provider used by Work product gates. It
// intentionally models only a bounded sequence of turns, not a legacy agent
// runtime or a provider transport.
type ScriptedModel struct {
	Turns []agent.Turn
}

func (m *ScriptedModel) Complete(_ context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	if m == nil || len(m.Turns) == 0 {
		return agent.Turn{}, errors.New("eval script is exhausted")
	}
	turn := m.Turns[0]
	m.Turns = m.Turns[1:]
	return turn, nil
}

func writeJSON(path string, value any, label string) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", label, err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create %s directory: %w", label, err)
	}
	temporary, err := os.CreateTemp(directory, ".gator-eval-report-")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", label, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("restrict temporary %s: %w", label, err)
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write %s: %w", label, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync %s: %w", label, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s: %w", label, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish %s: %w", label, err)
	}
	return nil
}
