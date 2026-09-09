package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// RootSet is a primary writable workspace plus explicitly granted read-only
// directories. Relative paths always resolve inside Primary. Absolute paths
// are accepted only for files below an additional root, which makes a caller
// opt in visibly when it leaves the worktree.
type RootSet struct {
	primary    Root
	additional []Root
	named      []NamedRoot
}

// NamedRoot exposes a root through a stable virtual path such as source/... or
// output/.... Naming affects resolution and display only; the tool that uses a
// RootSet still decides whether an operation is read-only or writable.
type NamedRoot struct {
	Name string
	Root Root
}

// FileRef is one regular file discovered through a descriptor-rooted walk.
// Relative is suitable for Root.ReadRegularFile; Path is the stable path to
// show a model or developer.
type FileRef struct {
	Root     Root
	Relative string
	Path     string
	Size     int64
}

// NewRootSet creates a stable boundary around a primary workspace and already
// canonicalized additional roots. Duplicate roots and the primary root are
// ignored. The caller is responsible for deciding which host paths to grant.
func NewRootSet(primary Root, additional []Root) RootSet {
	result, _ := NewNamedRootSet(primary, additional, nil)
	return result
}

// NewNamedRootSet adds stable virtual names without exposing canonical host
// paths to a model. Names are validated and sorted for deterministic display.
func NewNamedRootSet(primary Root, additional []Root, named []NamedRoot) (RootSet, error) {
	seen := map[string]struct{}{primary.Path(): {}}
	result := RootSet{primary: primary, additional: make([]Root, 0, len(additional)), named: make([]NamedRoot, 0, len(named))}
	for _, root := range additional {
		if root.Path() == "" {
			continue
		}
		if _, found := seen[root.Path()]; found {
			continue
		}
		seen[root.Path()] = struct{}{}
		result.additional = append(result.additional, root)
	}
	sort.Slice(result.additional, func(first, second int) bool {
		return result.additional[first].Path() < result.additional[second].Path()
	})
	seenNames := make(map[string]struct{}, len(named))
	for _, item := range named {
		if !validRootName(item.Name) {
			return RootSet{}, fmt.Errorf("invalid named workspace root %q", item.Name)
		}
		if item.Root.Path() == "" {
			return RootSet{}, fmt.Errorf("named workspace root %q is not initialized", item.Name)
		}
		if _, duplicate := seenNames[item.Name]; duplicate {
			return RootSet{}, fmt.Errorf("named workspace root %q is repeated", item.Name)
		}
		seenNames[item.Name] = struct{}{}
		result.named = append(result.named, item)
	}
	sort.Slice(result.named, func(first, second int) bool {
		if len(result.named[first].Root.Path()) != len(result.named[second].Root.Path()) {
			return len(result.named[first].Root.Path()) > len(result.named[second].Root.Path())
		}
		return result.named[first].Name < result.named[second].Name
	})
	return result, nil
}

// OpenAdditionalRoots validates and canonicalizes absolute existing
// directories supplied by an integration boundary. The filesystem root is
// deliberately never a usable additional root.
func OpenAdditionalRoots(paths []string, maximum int) ([]Root, error) {
	if maximum > 0 && len(paths) > maximum {
		return nil, fmt.Errorf("at most %d additional workspace roots are allowed", maximum)
	}
	result := make([]Root, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("additional workspace root %q must be an absolute path", path)
		}
		root, err := Open(path)
		if err != nil {
			return nil, fmt.Errorf("open additional workspace root %q: %w", path, err)
		}
		if root.Path() == string(filepath.Separator) {
			return nil, errors.New("additional workspace root must not be the filesystem root")
		}
		if _, duplicate := seen[root.Path()]; duplicate {
			continue
		}
		seen[root.Path()] = struct{}{}
		result = append(result, root)
	}
	return result, nil
}

func (r RootSet) Primary() Root { return r.primary }

// Additional returns an independent copy of the read-only roots.
func (r RootSet) Additional() []Root { return append([]Root(nil), r.additional...) }

// Named returns an independent copy of virtual workspace roots.
func (r RootSet) Named() []NamedRoot { return append([]NamedRoot(nil), r.named...) }

// Paths returns the primary root followed by canonical additional roots. It
// is intended for protocol workspace-folder lists and cache identities.
func (r RootSet) Paths() []string {
	result := make([]string, 0, len(r.additional)+len(r.named)+1)
	seen := make(map[string]struct{}, len(r.additional)+len(r.named)+1)
	if r.primary.Path() != "" {
		result = append(result, r.primary.Path())
		seen[r.primary.Path()] = struct{}{}
	}
	for _, root := range r.additional {
		if _, duplicate := seen[root.Path()]; !duplicate {
			result = append(result, root.Path())
			seen[root.Path()] = struct{}{}
		}
	}
	for _, item := range r.named {
		if _, duplicate := seen[item.Root.Path()]; !duplicate {
			result = append(result, item.Root.Path())
			seen[item.Root.Path()] = struct{}{}
		}
	}
	return result
}

// ResolveFile resolves a primary-relative or additional-root absolute path.
func (r RootSet) ResolveFile(path string) (string, error) {
	if !filepath.IsAbs(path) {
		if root, relative, found := r.namedFor(path); found {
			if relative == "." {
				return "", fmt.Errorf("named workspace path %q identifies a directory, not a file", path)
			}
			return root.ResolveFile(relative)
		}
		return r.primary.ResolveFile(path)
	}
	root, relative, found := r.additionalFor(path, false)
	if !found || relative == "." {
		return "", fmt.Errorf("absolute path %q is outside the read-only workspace roots", path)
	}
	return root.ResolveFile(relative)
}

// ReadRegularFile reads through the selected descriptor-rooted workspace and
// returns the stable display path. It avoids reopening a previously resolved
// host path, closing a symlink replacement race in file-oriented tools.
func (r RootSet) ReadRegularFile(value string, maxBytes int64) ([]byte, string, error) {
	root, relative, display, err := r.selectFile(value)
	if err != nil {
		return nil, "", err
	}
	contents, err := root.ReadRegularFile(relative, maxBytes)
	if err != nil {
		return nil, "", err
	}
	return contents, display, nil
}

// WalkRegularFiles traverses one selected workspace directory without
// following symlinks. All opens remain rooted beneath the selected descriptor.
func (r RootSet) WalkRegularFiles(value string, skipDirectory func(string) bool, visit func(FileRef) error) error {
	if visit == nil {
		return errors.New("workspace file visitor is required")
	}
	root, relative, prefix, err := r.selectDirectory(value)
	if err != nil {
		return err
	}
	descriptor, err := os.OpenRoot(root.Path())
	if err != nil {
		return fmt.Errorf("open workspace root: %w", err)
	}
	defer descriptor.Close()
	directory, err := descriptor.OpenRoot(relative)
	if err != nil {
		return fmt.Errorf("open workspace directory %q: %w", value, err)
	}
	defer directory.Close()
	err = fs.WalkDir(directory.FS(), ".", func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if current != "." && skipDirectory != nil && skipDirectory(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fullRelative := path.Join(filepath.ToSlash(relative), current)
		if relative == "." {
			fullRelative = current
		}
		display := fullRelative
		if prefix != "" {
			display = path.Join(prefix, fullRelative)
		}
		return visit(FileRef{Root: root, Relative: filepath.FromSlash(fullRelative), Path: display, Size: info.Size()})
	})
	if errors.Is(err, fs.SkipAll) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("walk workspace files: %w", err)
	}
	return nil
}

// ResolveDirectory resolves a list/search directory. An empty path denotes
// the primary workspace; an additional root itself may be named absolutely.
func (r RootSet) ResolveDirectory(path string) (string, error) {
	if path == "" || path == "." {
		return r.primary.Path(), nil
	}
	if !filepath.IsAbs(path) {
		if root, relative, found := r.namedFor(path); found {
			if relative == "." {
				return root.Path(), nil
			}
			return root.ResolveFile(relative)
		}
	}
	if filepath.IsAbs(path) {
		root, relative, found := r.additionalFor(path, false)
		if !found {
			return "", fmt.Errorf("absolute path %q is outside the read-only workspace roots", path)
		}
		if relative == "." {
			return root.Path(), nil
		}
		return root.ResolveFile(relative)
	}
	return r.primary.ResolveFile(path)
}

// ResolveReturnedFile resolves an absolute file URI path reported by a trusted
// LSP server. Unlike ResolveFile, it also accepts the primary root because
// protocol responses necessarily use absolute file URIs.
func (r RootSet) ResolveReturnedFile(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("returned workspace path %q must be absolute", path)
	}
	root, relative, found := r.additionalFor(path, true)
	if !found || relative == "." {
		return "", fmt.Errorf("returned path %q is outside the workspace roots", path)
	}
	return root.ResolveFile(relative)
}

// DisplayPath preserves the established relative convention for the primary
// worktree and uses a canonical absolute path for an additional root.
func (r RootSet) DisplayPath(path string) string {
	for _, item := range r.named {
		if relative, inside := relativeToRoot(item.Root, path); inside {
			if relative == "." {
				return item.Name
			}
			return filepath.ToSlash(filepath.Join(item.Name, relative))
		}
	}
	if relative, inside := relativeToRoot(r.primary, path); inside {
		return filepath.ToSlash(relative)
	}
	return path
}

func (r RootSet) namedFor(value string) (Root, string, bool) {
	clean := filepath.Clean(value)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return Root{}, "", false
	}
	for _, item := range r.named {
		if clean == item.Name {
			return item.Root, ".", true
		}
		prefix := item.Name + string(filepath.Separator)
		if strings.HasPrefix(clean, prefix) {
			return item.Root, strings.TrimPrefix(clean, prefix), true
		}
	}
	return Root{}, "", false
}

func (r RootSet) selectFile(value string) (Root, string, string, error) {
	if !filepath.IsAbs(value) {
		if root, relative, found := r.namedFor(value); found {
			if relative == "." {
				return Root{}, "", "", fmt.Errorf("named workspace path %q identifies a directory, not a file", value)
			}
			clean := filepath.Clean(value)
			return root, relative, filepath.ToSlash(clean), nil
		}
		clean, err := cleanRelativePath(value)
		if err != nil {
			return Root{}, "", "", err
		}
		return r.primary, clean, filepath.ToSlash(clean), nil
	}
	root, relative, found := r.additionalFor(value, false)
	if !found || relative == "." {
		return Root{}, "", "", fmt.Errorf("absolute path %q is outside the read-only workspace roots", value)
	}
	display := filepath.Join(root.Path(), relative)
	return root, relative, display, nil
}

func (r RootSet) selectDirectory(value string) (Root, string, string, error) {
	if value == "" || value == "." {
		return r.primary, ".", "", nil
	}
	if !filepath.IsAbs(value) {
		if root, relative, found := r.namedFor(value); found {
			prefix := filepath.ToSlash(filepath.Clean(value))
			if relative != "." {
				prefix = strings.TrimSuffix(prefix, "/"+filepath.ToSlash(relative))
			}
			return root, relative, prefix, nil
		}
		clean, err := cleanRelativePath(value)
		if err != nil {
			return Root{}, "", "", err
		}
		return r.primary, clean, "", nil
	}
	root, relative, found := r.additionalFor(value, false)
	if !found {
		return Root{}, "", "", fmt.Errorf("absolute path %q is outside the read-only workspace roots", value)
	}
	return root, relative, filepath.ToSlash(root.Path()), nil
}

func validRootName(value string) bool {
	if len(value) == 0 || len(value) > 32 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' && index > 0 || (character == '-' || character == '_') && index > 0 {
			continue
		}
		return false
	}
	return true
}

// IsPrimary reports whether a resolved path is within the writable primary
// worktree. It is useful for consumers that may inspect external roots but
// must never return edit proposals for them.
func (r RootSet) IsPrimary(path string) bool {
	_, inside := relativeToRoot(r.primary, path)
	return inside
}

func (r RootSet) additionalFor(path string, includePrimary bool) (Root, string, bool) {
	if includePrimary {
		if relative, inside := relativeToRoot(r.primary, path); inside {
			return r.primary, relative, true
		}
	}
	for _, root := range r.additional {
		if relative, inside := relativeToRoot(root, path); inside {
			return root, relative, true
		}
	}
	return Root{}, "", false
}

func relativeToRoot(root Root, path string) (string, bool) {
	if root.Path() == "" {
		return "", false
	}
	cleaned := filepath.Clean(path)
	relative, err := filepath.Rel(root.Path(), cleaned)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", false
	}
	if relative == "." {
		return relative, true
	}
	info, err := os.Lstat(cleaned)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	return relative, true
}
