package worksession

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/workspace"
)

// BranchInfo records provenance, never an execution/authorization inheritance.
// Files remain immutable references; copied turns contain no native replay,
// approvals, action receipts, worker identity or tool-call/result fragments.
type BranchInfo struct {
	Version              int                 `json:"version"`
	ParentConversationID string              `json:"parent_conversation_id"`
	ParentRevisionID     string              `json:"parent_revision_id"`
	CreatedAt            time.Time           `json:"created_at"`
	Revisions            []InheritedRevision `json:"revisions"`
}
type InheritedRevision struct {
	RevisionID       string `json:"revision_id"`
	ConversationID   string `json:"conversation_id"`
	SourceRevisionID string `json:"source_revision_id"`
	SHA256           string `json:"sha256"`
	RunID            string `json:"run_id"`
}
type forkSeed struct {
	info   BranchInfo
	turns  []Revision
	digest string
}

// ForkChatOnce uses the existing cross-process creation receipt and atomic
// directory publication. Matching retries return the original chat; deletion
// cannot resurrect it. The caller holds the same profile guard as chat creation.
func (s Store) ForkChatOnce(parent, revision, run, requestID string, now time.Time) (Conversation, error) {
	c, err := s.Load(parent)
	if err != nil {
		return Conversation{}, err
	}
	turns, err := s.Lineage(parent, revision)
	if err != nil || len(turns) == 0 || len(turns) > 256 {
		return Conversation{}, errors.New("the selected retained boundary is unavailable or exceeds 256 turns")
	}
	selected := turns[len(turns)-1]
	if selected.Status != "completed" || selected.FinalText == "" || filepath.Base(selected.BundlePath) != run {
		return Conversation{}, errors.New("branching requires an exact completed retained answer")
	}
	inherited, err := s.Branch(parent)
	if err != nil {
		return Conversation{}, err
	}
	seed := &forkSeed{turns: turns, info: BranchInfo{Version: 1, ParentConversationID: parent, ParentRevisionID: revision}}
	root, err := workspace.Open(s.root)
	if err != nil {
		return Conversation{}, err
	}
	digest := sha256.New()
	total := 0
	for _, turn := range turns {
		raw, err := root.ReadRegularFile(filepath.Join(parent, "revisions", turn.ID+".json"), 8*1024*1024)
		if err != nil {
			return Conversation{}, fmt.Errorf("retained branch context unavailable: %w", err)
		}
		total += len(raw)
		if total > 32*1024*1024 {
			return Conversation{}, errors.New("branch context exceeds 32 MiB")
		}
		digest.Write(raw)
		sum := sha256.Sum256(raw)
		origin := InheritedRevision{RevisionID: turn.ID, ConversationID: parent, SourceRevisionID: turn.ID, SHA256: hex.EncodeToString(sum[:]), RunID: filepath.Base(turn.BundlePath)}
		if inherited != nil {
			for _, prior := range inherited.Revisions {
				if prior.RevisionID == turn.ID {
					origin = prior
					origin.RevisionID = turn.ID
					break
				}
			}
		}
		seed.info.Revisions = append(seed.info.Revisions, origin)
	}
	seed.digest = hex.EncodeToString(digest.Sum(nil))
	return s.createChatOnce(c.SourcePath, requestID, now, seed)
}

func (f *forkSeed) write(root *os.Root, stage string, c *Conversation) error {
	info := f.info
	info.CreatedAt = c.CreatedAt
	info.Revisions = append([]InheritedRevision(nil), f.info.Revisions...)
	parent := ""
	for i, original := range f.turns {
		sum := sha256.Sum256([]byte(c.ID + "\x00" + original.ID))
		id := "rev-branch-" + hex.EncodeToString(sum[:16])
		// Copy only complete visible turns. Dropping entire historical tool groups
		// avoids dangling calls and cannot carry consumed decisions into new work.
		turn := Revision{Version: Version, ID: id, ConversationID: c.ID, ParentRevisionID: parent, SnapshotID: original.SnapshotID, Objective: original.Objective, FinalText: original.FinalText, BundlePath: original.BundlePath, Status: original.Status, CreatedAt: original.CreatedAt}
		if err := writeCreationJSON(root, filepath.Join(stage, "revisions", id+".json"), turn); err != nil {
			return err
		}
		if original.Activity != nil && original.Activity.ValidDetails() {
			if err := creationDirectory(root, filepath.Join(stage, "activity")); err != nil {
				return err
			}
			if err := writeCreationJSON(root, filepath.Join(stage, "activity", id+".json"), original.Activity); err != nil {
				return err
			}
		}
		if len(original.InputNames) > 0 || len(original.InputIDs) > 0 {
			if err := creationDirectory(root, filepath.Join(stage, "input-files")); err != nil {
				return err
			}
			if len(original.InputNames) > 0 {
				if err := writeCreationJSON(root, filepath.Join(stage, "input-files", id+".json"), original.InputNames); err != nil {
					return err
				}
			}
			if len(original.InputIDs) > 0 {
				if err := writeCreationJSON(root, filepath.Join(stage, "input-files", id+".json.refs"), original.InputIDs); err != nil {
					return err
				}
			}
		}
		info.Revisions[i].RevisionID = id
		parent = id
		c.SnapshotID = turn.SnapshotID
	}
	c.HeadRevision = parent
	c.Title = "Branch · " + chatRequestTitle(f.turns[len(f.turns)-1].Objective)
	if len(c.Title) > 256 {
		c.Title = "Branch"
	}
	// New requests start without selected external accounts. Profile model,
	// speech and privacy preferences remain governed by current host settings.
	empty := []string{}
	if err := writeCreationJSON(root, filepath.Join(stage, "preferences.json"), Preferences{ConnectorIDs: &empty}); err != nil {
		return err
	}
	return writeCreationJSON(root, filepath.Join(stage, "branch.json"), info)
}

func (s Store) Branch(id string) (*BranchInfo, error) {
	if !catalogIdentity.MatchString(id) {
		return nil, errors.New("invalid branch identity")
	}
	root, err := workspace.Open(s.root)
	if err != nil {
		return nil, err
	}
	raw, err := root.ReadRegularFile(filepath.Join(id, "branch.json"), 256*1024)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var info BranchInfo
	if json.Unmarshal(raw, &info) != nil || info.Version != 1 || !catalogIdentity.MatchString(info.ParentConversationID) || !catalogIdentity.MatchString(info.ParentRevisionID) || info.CreatedAt.IsZero() || len(info.Revisions) == 0 || len(info.Revisions) > 256 {
		return nil, errors.New("invalid retained branch provenance")
	}
	seen := map[string]bool{}
	for _, origin := range info.Revisions {
		if !catalogIdentity.MatchString(origin.RevisionID) || !catalogIdentity.MatchString(origin.ConversationID) || !catalogIdentity.MatchString(origin.SourceRevisionID) || !catalogIdentity.MatchString(origin.RunID) || len(origin.SHA256) != 64 || strings.Trim(origin.SHA256, "0123456789abcdef") != "" || seen[origin.RevisionID] {
			return nil, errors.New("invalid inherited revision provenance")
		}
		seen[origin.RevisionID] = true
	}
	return &info, nil
}
