package workapp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// NativeWorkspaceAction resolves data, never terminal command text. The host
// consumes the confirmed source as a new managed owner, not a source mutation.
type NativeWorkspaceAction struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
}

type NativeWorkspaceResult struct {
	RequestID      string `json:"request_id"`
	ConversationID string `json:"conversation_id"`
	Source         string `json:"source,omitempty"`
	New            bool   `json:"new"`
	Error          string `json:"error,omitempty"`
}

type nativeWorkspaceResolved struct{ Result NativeWorkspaceResult }

func (m Model) ResolveNativeWorkspace(a NativeWorkspaceAction) (tea.Model, tea.Cmd) {
	if !m.config.ManagedSessions || len(a.RequestID) == 0 || len(a.RequestID) > 128 || m.nativeWorkspacePending != "" {
		return m, nil
	}
	if m.nativeWorkspaceResult != nil && m.nativeWorkspaceResult.RequestID == a.RequestID {
		return m, nil
	}
	m.nativeWorkspacePending = a.RequestID
	m.nativeWorkspaceResult = nil
	origin, previous, resolve := m.conversation, m.source, m.config.ResolveSource
	return m, func() tea.Msg {
		r := NativeWorkspaceResult{RequestID: a.RequestID, ConversationID: origin}
		var err error
		switch {
		case resolve == nil:
			err = fmt.Errorf("workspace selection is unavailable")
		case len(a.Path) > 8192 || !filepath.IsAbs(a.Path):
			err = fmt.Errorf("choose an absolute workspace directory")
		default:
			for _, c := range a.Path {
				if unicode.IsControl(c) {
					err = fmt.Errorf("workspace paths containing control characters are unsupported")
					break
				}
			}
			if err == nil {
				r.Source, err = resolve(a.Path)
			}
			if err == nil {
				// Open uses the canonical resolver. Probe directory enumeration too:
				// stat alone can succeed when this process cannot read the folder.
				var directory *os.File
				// resolving the final dot also requires directory traversal access.
				directory, err = os.Open(r.Source + string(os.PathSeparator) + ".")
				if err == nil {
					_, err = directory.Readdirnames(1)
					if err == io.EOF {
						err = nil
					}
					closeErr := directory.Close()
					if err == nil {
						err = closeErr
					}
				}
			}
		}
		if err != nil {
			r.Source = ""
			r.Error = err.Error()
		} else {
			r.New = r.Source != previous
			if r.New {
				// macOS panel/env strings can use different Unicode forms.
				// Directory identity stays in Go; do not infer it in Swift.
				selected, selectedErr := os.Stat(r.Source)
				current, currentErr := os.Stat(previous)
				if selectedErr == nil && currentErr == nil && current.IsDir() && os.SameFile(selected, current) {
					r.Source, r.New = previous, false
				}
			}
		}
		return nativeWorkspaceResolved{Result: r}
	}
}

func (m Model) finishNativeWorkspace(r NativeWorkspaceResult) (tea.Model, tea.Cmd) {
	if r.RequestID != m.nativeWorkspacePending {
		return m, nil
	}
	m.nativeWorkspacePending = ""
	if r.ConversationID != m.conversation {
		r.Source, r.New, r.Error = "", false, "the requesting conversation changed; choose the workspace again"
	}
	m.nativeWorkspaceResult = &r
	return m, nil
}
