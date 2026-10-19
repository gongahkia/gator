package lifecycle

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SourceMap struct {
	From string
	To   string
}
type RestoreSource struct {
	Original    string
	Destination string
	Available   bool
}
type RestorePlan struct {
	ID              string
	CreatedAt       time.Time
	Archive         string
	ArchiveSHA256   string
	Origin          string
	Target          string
	Files           int
	Bytes           int64
	Mappings        []SourceMap
	Sources         []RestoreSource
	PausedChecks    int
	InterruptedRuns int
	Notes           []string
}
type RestoreReceipt struct {
	Plan       RestorePlan
	RestoredAt time.Time
}

func restoreID(p RestorePlan) string {
	p.ID = ""
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func newRestoreTarget(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil || strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("choose a new absolute profile directory")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", fmt.Errorf("destination parent must exist: %w", err)
	}
	target := filepath.Join(parent, filepath.Base(absolute))
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("restore requires a new profile directory; existing paths are never merged or replaced")
	}
	return target, nil
}

// PreviewRestore verifies and examines a private staging copy. It leaves the
// destination untouched and binds the review to exact archive bytes and maps.
func (s Store) PreviewRestore(ctx context.Context, archive, target string, mappings []SourceMap) (RestorePlan, error) {
	var plan RestorePlan
	target, err := newRestoreTarget(target)
	if err != nil {
		return plan, err
	}
	if withinPath(s.stateDir, target) || withinPath(target, s.stateDir) {
		return plan, errors.New("restore into a separate profile outside the current profile")
	}
	archive, err = filepath.Abs(archive)
	if err != nil {
		return plan, err
	}
	mappings, err = normalizeMappings(mappings, target)
	if err != nil {
		return plan, err
	}
	stage, manifest, digest, err := stageBackup(ctx, archive, s.control)
	if stage != "" {
		defer os.RemoveAll(stage)
	}
	if err != nil {
		return plan, err
	}
	if !filepath.IsAbs(manifest.StateDir) || filepath.Clean(manifest.StateDir) != manifest.StateDir || manifest.StateDir == string(filepath.Separator) {
		return plan, errors.New("backup has no valid original state directory")
	}
	plan = RestorePlan{CreatedAt: time.Now().UTC(), Archive: archive, ArchiveSHA256: digest, Origin: manifest.StateDir, Target: target, Files: len(manifest.Files), Mappings: mappings,
		Notes: []string{
			"Creates a separate profile. The current profile, archive, and external projects remain unchanged.",
			"All restored checks are paused. New model requests start paused; credentials and provider configuration must be set up separately.",
			"Interrupted runs become failed historical records. Pending external approvals and runtime access are not resumed.",
			"Only navigation metadata is rebased. Artifact bytes, transcript text, frozen project configuration, and evidence hashes remain historical.",
			"Missing external sources remain listed for review. Map them to existing directories before restoring if you want to resume those projects immediately.",
		}}
	items, err := os.ReadDir(filepath.Join(stage, "profile"))
	if err != nil || len(items) == 0 {
		return RestorePlan{}, errors.New("backup contains no restorable profile")
	}
	plan.Sources, plan.PausedChecks, plan.InterruptedRuns, err = migrateProfile(ctx, filepath.Join(stage, "profile"), plan, false)
	if err != nil {
		return RestorePlan{}, err
	}
	for _, source := range plan.Sources {
		if !withinPath(target, source.Destination) && withinPath(source.Destination, target) {
			return RestorePlan{}, errors.New("new profile must be outside the selected external source folders")
		}
	}
	for _, mapping := range plan.Mappings {
		used := false
		for _, source := range plan.Sources {
			comparable := canonicalSourcePath(source.Original)
			if withinPath(mapping.From, comparable) {
				used = true
				break
			}
		}
		if !used {
			return RestorePlan{}, fmt.Errorf("source mapping %s does not match a source in this archive", mapping.From)
		}
	}
	if err = validateRestoredProfile(ctx, filepath.Join(stage, "profile"), manifest.StateDir); err != nil {
		return RestorePlan{}, fmt.Errorf("backup cannot be restored: %w", err)
	}
	for _, name := range sortedManifestFiles(manifest.Files) {
		info, err := os.Stat(filepath.Join(stage, "profile", filepath.FromSlash(name)))
		if err != nil {
			return RestorePlan{}, err
		}
		plan.Bytes += info.Size()
	}
	plan.ID = restoreID(plan)
	return plan, privateJSON(filepath.Join(s.control, "plans", "restore-"+plan.ID+".json"), plan)
}
func (s Store) LoadRestorePlan(id string) (RestorePlan, error) {
	var p RestorePlan
	if len(id) != 64 {
		return p, errors.New("invalid restore review ID")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return p, errors.New("invalid restore review ID")
	}
	data, err := os.ReadFile(filepath.Join(s.control, "plans", "restore-"+id+".json"))
	if err != nil {
		return p, err
	}
	if len(data) > 16<<20 {
		return p, errors.New("restore review is too large")
	}
	if err = json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	if p.ID != id || restoreID(p) != id || time.Since(p.CreatedAt) > time.Hour || p.CreatedAt.After(time.Now().Add(time.Minute)) {
		return p, errors.New("restore review changed or expired; preview again")
	}
	return p, nil
}

// Restore publishes only after extraction, migration, validation and fsync all
// succeed. The final rename is exclusive on both supported platforms.
func (s Store) Restore(ctx context.Context, id string) (RestoreReceipt, error) {
	var receipt RestoreReceipt
	p, err := s.LoadRestorePlan(id)
	if err != nil {
		return receipt, err
	}
	target, err := newRestoreTarget(p.Target)
	if err != nil {
		return receipt, err
	}
	if target != p.Target {
		return receipt, errors.New("destination changed during review; preview again")
	}
	mappings, err := normalizeMappings(p.Mappings, target)
	if err != nil {
		return receipt, err
	}
	if !sameMappings(mappings, p.Mappings) {
		return receipt, errors.New("source mapping changed during review; preview again")
	}
	parent, err := os.OpenRoot(filepath.Dir(target))
	if err != nil {
		return receipt, err
	}
	defer parent.Close()
	stage, manifest, digest, err := stageBackup(ctx, p.Archive, filepath.Dir(target))
	if stage != "" {
		defer os.RemoveAll(stage)
	}
	if err != nil {
		return receipt, err
	}
	if digest != p.ArchiveSHA256 || manifest.StateDir != p.Origin || len(manifest.Files) != p.Files {
		return receipt, errors.New("backup changed after review; preview again")
	}
	profile := filepath.Join(stage, "profile")
	sources, checks, runs, err := migrateProfile(ctx, profile, p, true)
	if err != nil {
		return receipt, err
	}
	if checks != p.PausedChecks || runs != p.InterruptedRuns || !sameSources(sources, p.Sources) {
		return receipt, errors.New("source availability changed after review; preview again")
	}
	if err = validateRestoredProfile(ctx, profile, p.Target); err != nil {
		return receipt, err
	}
	receipt = RestoreReceipt{Plan: p, RestoredAt: time.Now().UTC()}
	control := filepath.Join(profile, "gator", "data-control")
	if err = os.MkdirAll(control, 0700); err != nil {
		return RestoreReceipt{}, err
	}
	if err = privateJSON(filepath.Join(control, "restored.json"), receipt); err != nil {
		return RestoreReceipt{}, err
	}
	// A migrated profile never inherits runnable background behavior or retention.
	usage := filepath.Join(profile, "gator", "usage")
	if err = os.MkdirAll(usage, 0700); err != nil {
		return RestoreReceipt{}, err
	}
	if err = privateJSON(filepath.Join(usage, "policy.json"), map[string]any{"paused": true}); err != nil {
		return RestoreReceipt{}, err
	}
	if err = syncProfile(ctx, profile); err != nil {
		return RestoreReceipt{}, err
	}
	if err = ctx.Err(); err != nil {
		return RestoreReceipt{}, err
	}
	if err = publishProfile(parent, filepath.Join(filepath.Base(stage), "profile"), filepath.Base(target)); err != nil {
		return RestoreReceipt{}, fmt.Errorf("publish fresh profile without replacing an existing path: %w", err)
	}
	// Publication succeeded even if the final parent sync is unavailable.
	if dir, err := parent.Open("."); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return receipt, nil
}

func stageBackup(ctx context.Context, archive, parent string) (stage string, manifest BackupManifest, digest string, err error) {
	stage, err = os.MkdirTemp(parent, ".gator-restore-")
	if err != nil {
		return
	}
	source, err := os.Open(archive)
	if err != nil {
		return
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return
	}
	if !info.Mode().IsRegular() || info.Size() > 12<<30 {
		err = errors.New("backup must be a regular archive within 12 GiB")
		return
	}
	copyPath := filepath.Join(stage, "archive.tar.gz")
	copy, err := os.OpenFile(copyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(copy, hash), io.LimitReader(cancelReader{ctx, source}, (12<<30)+1))
	closeErr := copy.Close()
	if err != nil {
		return
	}
	if closeErr != nil {
		err = closeErr
		return
	}
	digest = hex.EncodeToString(hash.Sum(nil))
	profile := filepath.Join(stage, "profile")
	if err = os.Mkdir(profile, 0700); err != nil {
		return
	}
	root, err := os.OpenRoot(profile)
	if err != nil {
		return
	}
	defer root.Close()
	manifest, err = readBackup(ctx, copyPath, func(header *tar.Header, reader io.Reader) error {
		name := filepath.FromSlash(header.Name)
		if err := root.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}
		file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		n, err := io.Copy(file, reader)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if n != header.Size {
			return errors.New("backup entry was truncated")
		}
		if closeErr != nil {
			return closeErr
		}
		return nil
	})
	return
}
func sortedManifestFiles(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func syncProfile(ctx context.Context, profile string) error {
	var dirs []string
	err := filepath.WalkDir(profile, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		err = file.Sync()
		closeErr := file.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		dir, err := os.Open(dirs[i])
		if err != nil {
			return err
		}
		err = dir.Sync()
		dir.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
func sameMappings(a, b []SourceMap) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func sameSources(a, b []RestoreSource) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
