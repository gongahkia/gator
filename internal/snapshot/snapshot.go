// Package snapshot captures immutable, content-addressed evidence for Work sources.
package snapshot

import (
	"bufio"
	"crypto/rand"
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

const Version = 1

const (
	DefaultMaxFiles     = 50_000
	DefaultMaxTotal     = int64(1024 * 1024 * 1024)
	DefaultMaxFileBytes = int64(100 * 1024 * 1024)
)

// Limits bounds a snapshot before bytes are retained in Gator's private store.
type Limits struct {
	MaxFiles     int   `json:"max_files"`
	MaxTotal     int64 `json:"max_total_bytes"`
	MaxFileBytes int64 `json:"max_file_bytes"`
}

func DefaultLimits() Limits {
	return Limits{MaxFiles: DefaultMaxFiles, MaxTotal: DefaultMaxTotal, MaxFileBytes: DefaultMaxFileBytes}
}

func (l Limits) normalize() Limits {
	defaults := DefaultLimits()
	if l.MaxFiles == 0 {
		l.MaxFiles = defaults.MaxFiles
	}
	if l.MaxTotal == 0 {
		l.MaxTotal = defaults.MaxTotal
	}
	if l.MaxFileBytes == 0 {
		l.MaxFileBytes = defaults.MaxFileBytes
	}
	return l
}

func (l Limits) validate() error {
	if l.MaxFiles < 1 || l.MaxFiles > 1_000_000 || l.MaxTotal < 1 || l.MaxTotal > 100*1024*1024*1024 || l.MaxFileBytes < 1 || l.MaxFileBytes > l.MaxTotal {
		return errors.New("snapshot limits are invalid")
	}
	return nil
}

// Entry records one regular file. Paths always use slash separators.
type Entry struct {
	Path           string    `json:"path"`
	Bytes          int64     `json:"bytes"`
	Mode           uint32    `json:"mode"`
	SourceModified time.Time `json:"source_modified_at"`
	SHA256         string    `json:"sha256"`
}

// Exclusion makes omissions inspectable instead of silently producing a partial source.
type Exclusion struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Manifest is the portable identity of a frozen local source.
type Manifest struct {
	Version      int         `json:"version"`
	ID           string      `json:"id"`
	SourceName   string      `json:"source_name"`
	SourcePath   string      `json:"source_path"`
	CreatedAt    time.Time   `json:"created_at"`
	SHA256       string      `json:"sha256"`
	Files        int         `json:"files"`
	Bytes        int64       `json:"bytes"`
	Entries      []Entry     `json:"entries"`
	Exclusions   []Exclusion `json:"exclusions,omitempty"`
	Materialized string      `json:"-"`
}

// Options selects limits and additional gitignore-style path patterns.
type Options struct {
	Limits   Limits
	Excludes []string
	Now      func() time.Time
}

// LimitError reports the exact path and limit that prevented a full snapshot.
type LimitError struct {
	Path  string
	Limit string
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("snapshot %s limit exceeded at %q", e.Limit, e.Path)
}

var excludedDirectories = map[string]struct{}{
	".git": {}, ".gator": {}, "node_modules": {}, "vendor": {}, "target": {}, "dist": {}, "build": {},
	".cache": {}, ".next": {}, ".turbo": {}, "__pycache__": {}, ".venv": {}, "venv": {},
}

func secretName(name string) bool {
	lower := strings.ToLower(name)
	if lower == ".env" || strings.HasPrefix(lower, ".env.") || lower == "credentials.json" || lower == "id_rsa" || lower == "id_ed25519" {
		return true
	}
	return strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") || strings.HasSuffix(lower, ".p12") || strings.HasSuffix(lower, ".pfx")
}

// Create freezes source below stateDir/gator/snapshots and returns its manifest.
func Create(source, stateDir string, options Options) (Manifest, error) {
	limits := options.Limits.normalize()
	if err := limits.validate(); err != nil {
		return Manifest{}, err
	}
	sourcePath, err := filepath.EvalSymlinks(source)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve snapshot source: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil || !info.IsDir() {
		return Manifest{}, errors.New("snapshot source must be a directory")
	}
	statePath, err := filepath.Abs(stateDir)
	if err != nil || strings.TrimSpace(stateDir) == "" {
		return Manifest{}, errors.New("snapshot state directory is required")
	}
	if within(sourcePath, statePath) {
		return Manifest{}, errors.New("snapshot state directory must not be inside the source")
	}
	store := filepath.Join(statePath, "gator", "snapshots")
	for _, directory := range []string{store, filepath.Join(store, "blobs"), filepath.Join(store, "manifests"), filepath.Join(store, "trees")} {
		if err := ensureDir(directory, 0o700); err != nil {
			return Manifest{}, err
		}
	}
	patterns := append([]string(nil), options.Excludes...)
	patterns = append(patterns, readIgnoreFile(filepath.Join(sourcePath, ".gatorignore"))...)
	manifest := Manifest{Version: Version, SourceName: filepath.Base(sourcePath), SourcePath: sourcePath}
	if options.Now != nil {
		manifest.CreatedAt = options.Now().UTC()
	} else {
		manifest.CreatedAt = time.Now().UTC()
	}
	err = filepath.WalkDir(sourcePath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourcePath, path)
		if err != nil || relative == "." {
			return err
		}
		slash := filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			manifest.Exclusions = append(manifest.Exclusions, Exclusion{Path: slash, Reason: "symlink"})
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if _, excluded := excludedDirectories[entry.Name()]; excluded || matches(patterns, slash, true) {
				manifest.Exclusions = append(manifest.Exclusions, Exclusion{Path: slash + "/", Reason: "excluded directory"})
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			manifest.Exclusions = append(manifest.Exclusions, Exclusion{Path: slash, Reason: "not a regular file"})
			return nil
		}
		if secretName(entry.Name()) || matches(patterns, slash, false) {
			manifest.Exclusions = append(manifest.Exclusions, Exclusion{Path: slash, Reason: "excluded file"})
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if fileInfo.Size() > limits.MaxFileBytes {
			return &LimitError{Path: slash, Limit: "per-file"}
		}
		if len(manifest.Entries)+1 > limits.MaxFiles {
			return &LimitError{Path: slash, Limit: "file-count"}
		}
		if manifest.Bytes+fileInfo.Size() > limits.MaxTotal {
			return &LimitError{Path: slash, Limit: "total-size"}
		}
		digest, err := retainBlob(path, filepath.Join(store, "blobs"), limits.MaxFileBytes)
		if err != nil {
			return fmt.Errorf("snapshot %q: %w", slash, err)
		}
		manifest.Entries = append(manifest.Entries, Entry{Path: slash, Bytes: fileInfo.Size(), Mode: uint32(fileInfo.Mode().Perm()), SourceModified: fileInfo.ModTime().UTC(), SHA256: digest})
		manifest.Bytes += fileInfo.Size()
		return nil
	})
	if err != nil {
		return Manifest{}, fmt.Errorf("capture source snapshot: %w", err)
	}
	sort.Slice(manifest.Entries, func(i, j int) bool { return manifest.Entries[i].Path < manifest.Entries[j].Path })
	sort.Slice(manifest.Exclusions, func(i, j int) bool { return manifest.Exclusions[i].Path < manifest.Exclusions[j].Path })
	manifest.Files = len(manifest.Entries)
	manifest.SHA256, err = identity(manifest.Entries)
	if err != nil {
		return Manifest{}, err
	}
	manifest.ID = "snap-" + manifest.SHA256[:24]
	manifest.Materialized = filepath.Join(store, "trees", manifest.ID)
	if err := materialize(manifest, store); err != nil {
		return Manifest{}, err
	}
	if err := writeJSON(filepath.Join(store, "manifests", manifest.ID+".json"), manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Open validates a retained snapshot and returns its materialized read-only tree.
func Open(stateDir, id string) (Manifest, error) {
	if !strings.HasPrefix(id, "snap-") || strings.ContainsAny(id, `/\\`) {
		return Manifest{}, errors.New("invalid snapshot ID")
	}
	store := filepath.Join(stateDir, "gator", "snapshots")
	payload, err := os.ReadFile(filepath.Join(store, "manifests", id+".json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("read snapshot: %w", err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode snapshot: %w", err)
	}
	digest, err := identity(manifest.Entries)
	if err != nil || manifest.Version != Version || manifest.ID != id || digest != manifest.SHA256 || manifest.Files != len(manifest.Entries) {
		return Manifest{}, errors.New("snapshot manifest is invalid")
	}
	manifest.Materialized = filepath.Join(store, "trees", manifest.ID)
	if info, err := os.Stat(manifest.Materialized); err != nil || !info.IsDir() {
		return Manifest{}, errors.New("snapshot materialized tree is missing")
	}
	return manifest, nil
}

// List returns retained snapshot manifests in newest-first order.
func List(stateDir string) ([]Manifest, error) {
	directory := filepath.Join(stateDir, "gator", "snapshots", "manifests")
	files, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var manifests []Manifest
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		manifest, err := Open(stateDir, strings.TrimSuffix(file.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].CreatedAt.After(manifests[j].CreatedAt) })
	return manifests, nil
}

// GC removes snapshots not present in referenced, then removes unreachable blobs.
func GC(stateDir string, referenced map[string]struct{}) (snapshotsRemoved, blobsRemoved int, err error) {
	manifests, err := List(stateDir)
	if err != nil {
		return 0, 0, err
	}
	store := filepath.Join(stateDir, "gator", "snapshots")
	for _, manifest := range manifests {
		if _, keep := referenced[manifest.ID]; keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(store, "trees", manifest.ID)); err != nil {
			return snapshotsRemoved, blobsRemoved, err
		}
		if err := os.Remove(filepath.Join(store, "manifests", manifest.ID+".json")); err != nil {
			return snapshotsRemoved, blobsRemoved, err
		}
		snapshotsRemoved++
	}
	remaining, err := List(stateDir)
	if err != nil {
		return snapshotsRemoved, blobsRemoved, err
	}
	live := make(map[string]struct{})
	for _, manifest := range remaining {
		for _, entry := range manifest.Entries {
			live[entry.SHA256] = struct{}{}
		}
	}
	blobs, err := os.ReadDir(filepath.Join(store, "blobs"))
	if errors.Is(err, os.ErrNotExist) {
		return snapshotsRemoved, 0, nil
	}
	if err != nil {
		return snapshotsRemoved, 0, err
	}
	for _, blob := range blobs {
		if blob.IsDir() {
			continue
		}
		if _, keep := live[blob.Name()]; !keep {
			if err := os.Remove(filepath.Join(store, "blobs", blob.Name())); err != nil {
				return snapshotsRemoved, blobsRemoved, err
			}
			blobsRemoved++
		}
	}
	return snapshotsRemoved, blobsRemoved, nil
}

func retainBlob(source, blobDir string, max int64) (string, error) {
	file, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer file.Close()
	temporary, err := os.CreateTemp(blobDir, ".blob-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o400); err != nil {
		_ = temporary.Close()
		return "", err
	}
	hash := sha256.New()
	read, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(file, max+1))
	if err != nil || read > max {
		_ = temporary.Close()
		if err == nil {
			err = errors.New("file changed while snapshotting and exceeds its limit")
		}
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	destination := filepath.Join(blobDir, digest)
	if err := os.Link(temporaryPath, destination); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	return digest, nil
}

func materialize(manifest Manifest, store string) error {
	destination := filepath.Join(store, "trees", manifest.ID)
	if info, err := os.Stat(destination); err == nil && info.IsDir() {
		return nil
	}
	temporary := destination + ".tmp-" + randomSuffix()
	if err := os.Mkdir(temporary, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	for _, entry := range manifest.Entries {
		target := filepath.Join(temporary, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.Link(filepath.Join(store, "blobs", entry.SHA256), target); err != nil {
			return err
		}
	}
	if err := os.Rename(temporary, destination); err != nil {
		if info, statErr := os.Stat(destination); statErr == nil && info.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

func identity(entries []Entry) (string, error) {
	hash := sha256.New()
	for _, entry := range entries {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(entry.Path)))
		if entry.Path == "" || filepath.IsAbs(entry.Path) || clean != entry.Path || clean == ".." || strings.HasPrefix(clean, "../") || len(entry.SHA256) != 64 || entry.Bytes < 0 {
			return "", errors.New("snapshot entry is invalid")
		}
		fmt.Fprintf(hash, "%s\x00%d\x00%d\x00%s\n", entry.Path, entry.Bytes, entry.Mode, entry.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readIgnoreFile(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var patterns []string
	scanner := bufio.NewScanner(io.LimitReader(file, 1024*1024))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "!") {
			patterns = append(patterns, strings.TrimPrefix(line, "/"))
		}
	}
	return patterns
}

func matches(patterns []string, path string, directory bool) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(strings.TrimPrefix(pattern, "/"))
		if pattern == "" {
			continue
		}
		if strings.HasSuffix(pattern, "/") {
			pattern = strings.TrimSuffix(pattern, "/")
			if path == pattern || strings.HasPrefix(path, pattern+"/") {
				return true
			}
			continue
		}
		if ok, _ := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(path)); ok {
			return true
		}
		if !strings.Contains(pattern, "/") {
			if ok, _ := filepath.Match(pattern, filepath.Base(path)); ok {
				return true
			}
		}
		if directory && (path == pattern || strings.HasPrefix(path, pattern+"/")) {
			return true
		}
	}
	return false
}

func ensureDir(path string, mode fs.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".manifest-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func randomSuffix() string {
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value)
}

func within(parent, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
