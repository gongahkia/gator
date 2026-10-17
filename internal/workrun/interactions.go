package workrun

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/gongahkia/gator/internal/workspace"
)

// InteractionDigest binds a decision to the displayed kind and full preview.
// The live operation retains the original digest, so editing the on-disk
// preview cannot turn a different request into an approved one.
func InteractionDigest(interaction Interaction) (string, error) {
	if interaction.ID < 1 || strings.TrimSpace(interaction.Kind) == "" {
		return "", errors.New("invalid Work interaction")
	}
	payload, err := json.Marshal(interaction)
	if err != nil {
		return "", err
	}
	if len(payload) > 1024*1024 {
		return "", errors.New("Work interaction preview is too large")
	}
	// Structs and maps can encode the same JSON fields in different orders.
	// Normalize both the original typed preview and the retained generic one.
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

const (
	maxInteractionRuns    = 4096
	maxInteractionRecords = 256
	maxInteractionScanned = 16384
	maxInteractionBytes   = 32 * 1024 * 1024
	interactionLockName   = "active.lock"
)

// acquireInteractionRunLock holds a kernel-released lease while a foreground
// Work run can ask for approvals. The lock file is never an approval channel.
func acquireInteractionRunLock(stateDir, runID string) (*os.File, error) {
	if stateDir == "" || !validInteractionRunID(runID) {
		return nil, errors.New("invalid Work interaction state")
	}
	base := filepath.Join(stateDir, "gator", "interactions")
	if err := os.MkdirAll(filepath.Join(base, runID), 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.OpenFile(filepath.Join(runID, interactionLockName), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("invalid Work interaction lock")
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("Work run is already active in another process")
		}
		return nil, err
	}
	return file, nil
}

func interactionRunLiveness(root *os.Root, runID string) (string, error) {
	file, err := root.OpenFile(filepath.Join(runID, interactionLockName), os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return "unknown", nil // older runs had no process-held lock
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("invalid Work interaction lock")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return "active", nil
		}
		return "", err
	}
	return "stopped", syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

// ReadInteractionRecords inspects retained decisions without contacting the
// original process or conferring authority to answer one from another process.
// An empty runID scans all retained runs. The caller must treat "pending" as
// recorded state rather than proof that a process is still waiting.
func ReadInteractionRecords(stateDir, runID string) ([]InteractionRecord, error) {
	return readInteractionRecords(stateDir, runID, false)
}

// ReadInteractionRecord loads one exact decision without scanning unrelated
// history. Its live status comes from the process-held interaction lock.
func ReadInteractionRecord(stateDir, runID string, id int) (InteractionRecord, error) {
	if !validInteractionRunID(runID) || id < 1 {
		return InteractionRecord{}, errors.New("invalid Work decision identity")
	}
	base := filepath.Join(stateDir, "gator", "interactions")
	root, err := workspace.Open(base)
	if err != nil {
		return InteractionRecord{}, err
	}
	liveRoot, err := os.OpenRoot(base)
	if err != nil {
		return InteractionRecord{}, err
	}
	defer liveRoot.Close()
	liveStatus, err := interactionRunLiveness(liveRoot, runID)
	if err != nil {
		return InteractionRecord{}, err
	}
	payload, err := root.ReadRegularFile(filepath.Join(runID, strconv.Itoa(id)+".json"), 1024*1024)
	if err != nil {
		return InteractionRecord{}, err
	}
	record, err := decodeInteractionRecord(payload, id)
	if err != nil {
		return InteractionRecord{}, err
	}
	record.RunID = runID
	record.LiveStatus = liveStatus
	return record, nil
}

// ReadPendingInteractionRecords scans retained runs but keeps only recorded
// pending decisions. Resolved history does not exhaust the result cap.
func ReadPendingInteractionRecords(stateDir string) ([]InteractionRecord, error) {
	return readInteractionRecords(stateDir, "", true)
}

func readInteractionRecords(stateDir, runID string, pendingOnly bool) ([]InteractionRecord, error) {
	if runID != "" && !validInteractionRunID(runID) {
		return nil, errors.New("invalid Work run ID")
	}
	base := filepath.Join(stateDir, "gator", "interactions")
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list Work interactions: %w", err)
	}
	if len(entries) > maxInteractionRuns {
		return nil, errors.New("too many retained Work runs to scan; specify a run ID")
	}
	root, err := workspace.Open(base)
	if err != nil {
		return nil, err
	}
	liveRoot, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	defer liveRoot.Close()
	var records []InteractionRecord
	var totalBytes int
	var scanned int
	for _, entry := range entries {
		name := entry.Name()
		if !validInteractionRunID(name) || !entry.IsDir() || (runID != "" && name != runID) {
			continue
		}
		liveStatus, err := interactionRunLiveness(liveRoot, name)
		if err != nil {
			return nil, fmt.Errorf("inspect Work run %s liveness: %w", name, err)
		}
		files, err := os.ReadDir(filepath.Join(base, name))
		if err != nil {
			return nil, fmt.Errorf("list decisions for %s: %w", name, err)
		}
		for _, file := range files {
			if !file.Type().IsRegular() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			id, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
			if err != nil || id < 1 {
				continue
			}
			scanned++
			if scanned > maxInteractionScanned {
				return nil, errors.New("too many retained decisions to scan; specify a run ID")
			}
			payload, err := root.ReadRegularFile(filepath.Join(name, file.Name()), 1024*1024)
			if err != nil {
				return nil, fmt.Errorf("read decision %s/%d: %w", name, id, err)
			}
			totalBytes += len(payload)
			if totalBytes > maxInteractionBytes {
				return nil, errors.New("retained decisions exceed the safe scan size; specify a run ID")
			}
			record, err := decodeInteractionRecord(payload, id)
			if err != nil {
				return nil, fmt.Errorf("decode decision %s/%d: %w", name, id, err)
			}
			record.RunID = name
			record.LiveStatus = liveStatus
			if !pendingOnly || record.Status == "pending" {
				if len(records) >= maxInteractionRecords {
					return nil, errors.New("too many matching decisions to show; specify a run ID")
				}
				records = append(records, record)
			}
		}
	}
	slices.SortFunc(records, func(a, b InteractionRecord) int {
		if a.RunID != b.RunID {
			return strings.Compare(b.RunID, a.RunID)
		}
		return b.Interaction.ID - a.Interaction.ID
	})
	return records, nil
}

func decodeInteractionRecord(payload []byte, id int) (InteractionRecord, error) {
	var record InteractionRecord
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&record); err != nil {
		return InteractionRecord{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return InteractionRecord{}, errors.New("invalid trailing decision data")
	}
	if record.Version != 1 || record.Interaction.ID != id || record.Interaction.Kind == "" ||
		!slices.Contains([]string{"pending", "resolved", "cancelled"}, record.Status) ||
		(record.Status == "resolved") != (record.Approved != nil) {
		return InteractionRecord{}, errors.New("invalid retained decision")
	}
	return record, nil
}

func validInteractionRunID(id string) bool {
	if len(id) < 1 || len(id) > 96 {
		return false
	}
	for i, c := range id {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || i > 0 && (c == '-' || c == '_') {
			continue
		}
		return false
	}
	return true
}
