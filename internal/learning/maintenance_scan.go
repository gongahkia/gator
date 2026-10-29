package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
)

func (s Store) maintenanceLock(ctx context.Context) (func(), error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := filelock.OpenRootFile(root, ".maintenance-lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err := filelock.Lock(file, filelock.Exclusive|filelock.Nonblocking); err == nil {
			return func() { _ = filelock.Unlock(file); _ = file.Close() }, nil
		} else if !errors.Is(err, filelock.ErrWouldBlock) {
			file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// A durable entry cursor rotates through a stable directory across passes.
// Directory mutations can shift entries, so every completed sweep starts over;
// per-observation identities make re-reading harmless. Payload reads remain
// bounded; skipping metadata also obeys the caller's time/cancellation budget.
func maintenanceWindow(ctx context.Context, directory *os.File, offset int) ([]os.DirEntry, error) {
	for remaining := offset; remaining > 0; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := directory.ReadDir(min(remaining, maintenanceScanLimit))
		remaining -= len(entries)
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(maintenanceScanLimit + 1)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return entries, err
}

type maintenanceGroup struct {
	ObservationIDs []string `json:"observation_ids"`
}

func observationGroup(observation Observation) (string, Scope) {
	scope := Scope{Kind: Global}
	if observation.Scope != nil {
		scope = *observation.Scope
	}
	return string(observation.Signal) + "\x00" + string(scope.Kind) + "\x00" + scope.Value, scope
}

func eligibleObservation(observation Observation, now time.Time) bool {
	return !observation.CreatedAt.Before(now.AddDate(0, 0, -30)) && !observation.CreatedAt.After(now) && len(observation.EvidenceRefs) != 0 && failureGuidance(observation.Signal) != ""
}

// Persist only three source IDs, never summaries, personal text or an inferred
// truth. Re-read each source before use so removal, expiry and scope changes
// invalidate accumulated evidence. Different pages can contribute independent
// events without multiplying retries from one Work.
func (s Store) accumulateMaintenance(observation Observation, now time.Time) ([]Observation, error) {
	key, _ := observationGroup(observation)
	digest := sha256.Sum256([]byte(key))
	directory := filepath.Join(s.root, ".maintenance-groups")
	if err := os.Mkdir(directory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("maintenance evidence directory must be ordinary")
	}
	if err := privatefs.Protect(directory); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, hex.EncodeToString(digest[:]))
	var retained maintenanceGroup
	if err := readJSONRecord(path, &retained); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(retained.ObservationIDs) > 3 {
		return nil, errors.New("maintenance evidence exceeds its bound")
	}
	workIDs := map[string]bool{}
	var observations []Observation
	for _, id := range append(retained.ObservationIDs, observation.ID) {
		current, err := s.LoadObservation(id)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		group, _ := observationGroup(current)
		if group != key || !eligibleObservation(current, now) || workIDs[current.WorkID] {
			continue
		}
		workIDs[current.WorkID] = true
		observations = append(observations, current)
		if len(observations) == 3 {
			break
		}
	}
	updated := maintenanceGroup{ObservationIDs: []string{}}
	for _, current := range observations {
		updated.ObservationIDs = append(updated.ObservationIDs, current.ID)
	}
	unchanged := len(updated.ObservationIDs) == len(retained.ObservationIDs)
	if unchanged {
		for i, id := range updated.ObservationIDs {
			unchanged = unchanged && id == retained.ObservationIDs[i]
		}
	}
	if !unchanged {
		if err := writeAtomic(path, updated); err != nil {
			return nil, err
		}
	}
	return observations, nil
}
