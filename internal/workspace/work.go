package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const workMetadataVersion = 1

var workIDPattern = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`)

// Work is one private non-Git work session. Source is developer-owned and
// read-only to agent tools. Output is the only durable writable artifact root;
// Scratch is never included in a deliverable.
type Work struct {
	ID           string
	Path         string
	Source       Root
	Output       Root
	Scratch      Root
	ManifestPath string
	CreatedAt    time.Time
}

type workMetadata struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateWork creates a private output workspace below an already-resolved
// state directory. It accepts an ordinary source directory and never invokes
// Git or copies source files into model-writable storage.
func CreateWork(sourcePath, stateDir, id string, now time.Time) (Work, error) {
	if !workIDPattern.MatchString(id) {
		return Work{}, fmt.Errorf("invalid work id %q", id)
	}
	if strings.TrimSpace(stateDir) == "" {
		return Work{}, errors.New("work state directory is required")
	}
	if now.IsZero() {
		return Work{}, errors.New("work creation time is required")
	}
	source, err := Open(sourcePath)
	if err != nil {
		return Work{}, fmt.Errorf("open work source: %w", err)
	}
	requestedState, err := canonicalFuturePath(stateDir)
	if err != nil {
		return Work{}, fmt.Errorf("resolve work state directory: %w", err)
	}
	if isWithin(source.Path(), requestedState) {
		return Work{}, errors.New("work state directory must not be inside the source")
	}
	stateRoot, err := privateStateRoot(stateDir)
	if err != nil {
		return Work{}, err
	}
	fingerprint := sha256.Sum256([]byte(source.Path()))
	gatorPath := filepath.Join(stateRoot, "gator")
	workspacePath := filepath.Join(gatorPath, "workspaces")
	parent := filepath.Join(workspacePath, hex.EncodeToString(fingerprint[:16]))
	for _, directory := range []string{gatorPath, workspacePath, parent} {
		if err := ensurePrivateDirectory(directory); err != nil {
			return Work{}, err
		}
	}
	runPath := filepath.Join(parent, id)
	if pathsOverlap(source.Path(), runPath) {
		return Work{}, errors.New("work source and private output workspace must not overlap")
	}
	if err := os.Mkdir(runPath, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Work{}, fmt.Errorf("work workspace already exists for %q", id)
		}
		return Work{}, fmt.Errorf("create work workspace: %w", err)
	}
	created := true
	defer func() {
		if created {
			_ = os.RemoveAll(runPath)
		}
	}()

	outputPath := filepath.Join(runPath, "output")
	scratchPath := filepath.Join(runPath, "scratch")
	for _, directory := range []string{outputPath, scratchPath} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			return Work{}, fmt.Errorf("create work workspace directory: %w", err)
		}
	}
	metadata := workMetadata{Version: workMetadataVersion, ID: id, Source: source.Path(), CreatedAt: now}
	if err := writePrivateJSON(filepath.Join(runPath, "workspace.json"), metadata); err != nil {
		return Work{}, err
	}
	work, err := openWork(runPath, metadata)
	if err != nil {
		return Work{}, err
	}
	created = false
	return work, nil
}

func canonicalFuturePath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	probe := absolute
	var missing []string
	for {
		_, err := os.Lstat(probe)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", fmt.Errorf("no existing ancestor for %q", value)
		}
		missing = append(missing, filepath.Base(probe))
		probe = parent
	}
	canonical, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return "", err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		canonical = filepath.Join(canonical, missing[index])
	}
	return canonical, nil
}

// OpenWork validates a retained non-Git workspace and its original source.
func OpenWork(path string) (Work, error) {
	if strings.TrimSpace(path) == "" {
		return Work{}, errors.New("work workspace path is required")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Work{}, fmt.Errorf("resolve work workspace: %w", err)
	}
	root, err := Open(canonical)
	if err != nil {
		return Work{}, fmt.Errorf("open work workspace root: %w", err)
	}
	contents, err := root.ReadRegularFile("workspace.json", 64*1024)
	if err != nil {
		return Work{}, fmt.Errorf("read work workspace metadata: %w", err)
	}
	var metadata workMetadata
	decoder := json.NewDecoder(strings.NewReader(string(contents)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return Work{}, fmt.Errorf("decode work workspace metadata: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return Work{}, errors.New("work workspace metadata contains more than one value")
		}
		return Work{}, fmt.Errorf("decode work workspace metadata: %w", err)
	}
	return openWork(canonical, metadata)
}

func openWork(runPath string, metadata workMetadata) (Work, error) {
	if metadata.Version != workMetadataVersion || !workIDPattern.MatchString(metadata.ID) || metadata.CreatedAt.IsZero() {
		return Work{}, errors.New("work workspace metadata is invalid")
	}
	if filepath.Base(runPath) != metadata.ID {
		return Work{}, errors.New("work workspace identity does not match its directory")
	}
	source, err := Open(metadata.Source)
	if err != nil {
		return Work{}, fmt.Errorf("reopen work source: %w", err)
	}
	if pathsOverlap(source.Path(), runPath) {
		return Work{}, errors.New("work source and private output workspace overlap")
	}
	output, err := Open(filepath.Join(runPath, "output"))
	if err != nil {
		return Work{}, fmt.Errorf("open work output: %w", err)
	}
	scratch, err := Open(filepath.Join(runPath, "scratch"))
	if err != nil {
		return Work{}, fmt.Errorf("open work scratch: %w", err)
	}
	if output.Path() == scratch.Path() {
		return Work{}, errors.New("work output and scratch roots overlap")
	}
	return Work{
		ID: metadata.ID, Path: runPath, Source: source, Output: output, Scratch: scratch,
		ManifestPath: filepath.Join(runPath, "manifest.json"), CreatedAt: metadata.CreatedAt,
	}, nil
}

func privateStateRoot(stateDir string) (string, error) {
	if strings.TrimSpace(stateDir) == "" {
		return "", errors.New("work state directory is required")
	}
	absolute, err := filepath.Abs(stateDir)
	if err != nil {
		return "", fmt.Errorf("resolve work state directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", fmt.Errorf("create work state directory: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonicalize work state directory: %w", err)
	}
	return canonical, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create private work directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat private work directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("private work path %q is not a directory", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("set private work directory permissions: %w", err)
	}
	return nil
}

func pathsOverlap(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if left == right {
		return true
	}
	leftToRight, leftErr := filepath.Rel(left, right)
	rightToLeft, rightErr := filepath.Rel(right, left)
	return (leftErr == nil && isDescendant(leftToRight)) || (rightErr == nil && isDescendant(rightToLeft))
}

func isWithin(parent, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(candidate))
	return err == nil && (relative == "." || isDescendant(relative))
}

func isDescendant(relative string) bool {
	return relative != "." && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func writePrivateJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode work workspace metadata: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create work workspace metadata: %w", err)
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("write work workspace metadata: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync work workspace metadata: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close work workspace metadata: %w", err)
	}
	return nil
}
