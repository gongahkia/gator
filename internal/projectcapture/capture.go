// Package projectcapture freezes selected project configuration separately from source data.
package projectcapture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/workspace"
)

const Version = 1
const maxBytes = 8 * 1024 * 1024

type File struct {
	Path string
	Data []byte
	Mode uint32
}
type Bundle struct {
	Version int
	Origin  string
	SHA256  string
	Files   []File
}

func Capture(source string, scopes []string, profile string, capabilities []string) (Bundle, error) {
	root, err := workspace.Open(source)
	if err != nil {
		return Bundle{}, err
	}
	guidance, err := instructions.LoadWithProfile(root.Path(), scopes, profile)
	if err != nil {
		return Bundle{}, err
	}
	selected := append([]string(nil), guidance.Files...)
	for _, name := range capabilities {
		switch name {
		case "hooks", "mcp", "lsp":
			selected = append(selected, ".gator/"+name+".json")
		case "extension":
			err := filepath.WalkDir(filepath.Join(root.Path(), ".gator", "extensions"), func(path string, entry fs.DirEntry, walkErr error) error {
				if errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				if walkErr != nil {
					return walkErr
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return errors.New("project extension contains symlink")
				}
				if !entry.IsDir() {
					relative, err := filepath.Rel(root.Path(), path)
					if err != nil {
						return err
					}
					selected = append(selected, filepath.ToSlash(relative))
				}
				return nil
			})
			if err != nil {
				return Bundle{}, err
			}
		}
	}
	bundle := Bundle{Version: Version, Origin: root.Path()}
	seen := map[string]bool{}
	total := 0
	for len(selected) > 0 {
		path := selected[0]
		selected = selected[1:]
		if !strings.HasPrefix(path, ".gator/") || seen[path] {
			continue
		}
		seen[path] = true
		lower := strings.ToLower(filepath.Base(path))
		if strings.Contains(lower, "credential") || lower == ".env" || strings.HasSuffix(lower, ".key") || strings.HasSuffix(lower, ".pem") {
			return Bundle{}, fmt.Errorf("configuration references credential file %q", path)
		}
		data, err := root.ReadRegularFile(path, maxBytes)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Bundle{}, err
		}
		total += len(data)
		if total > maxBytes || len(seen) > 512 {
			return Bundle{}, errors.New("captured configuration exceeds limit")
		}
		resolved, err := root.ResolveFile(path)
		if err != nil {
			return Bundle{}, err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return Bundle{}, err
		}
		bundle.Files = append(bundle.Files, File{path, data, uint32(info.Mode().Perm())})
		if filepath.Ext(path) == ".json" {
			var value any
			if err := json.Unmarshal(data, &value); err != nil {
				return Bundle{}, err
			}
			var visit func(any) error
			visit = func(v any) error {
				switch x := v.(type) {
				case map[string]any:
					for key, item := range x {
						switch strings.ToLower(key) {
						case "password", "access_token", "refresh_token", "authorization", "api_key":
							return fmt.Errorf("inline credential field %q cannot be captured", key)
						}
						if err := visit(item); err != nil {
							return err
						}
					}
				case []any:
					for _, item := range x {
						if err := visit(item); err != nil {
							return err
						}
					}
				case string:
					if strings.HasPrefix(x, ".gator/") {
						selected = append(selected, x)
					}
				}
				return nil
			}
			if err := visit(value); err != nil {
				return Bundle{}, err
			}
		}
	}
	sort.Slice(bundle.Files, func(i, j int) bool { return bundle.Files[i].Path < bundle.Files[j].Path })
	bundle.SHA256 = digest(bundle.Files)
	return bundle, nil
}

func digest(files []File) string {
	data, _ := json.Marshal(files)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func (b Bundle) Install(destination string) error {
	if b.Version != Version || b.Origin == "" || b.SHA256 != digest(b.Files) {
		return errors.New("captured project configuration identity is invalid")
	}
	root, err := workspace.Open(destination)
	if err != nil {
		return err
	}
	for _, file := range b.Files {
		if !strings.HasPrefix(file.Path, ".gator/") {
			return errors.New("captured configuration path is invalid")
		}
		if err := root.WriteRegularFileAtomic(file.Path, file.Data, maxBytes); err != nil {
			return err
		}
		path, err := root.ResolveFile(file.Path)
		if err != nil {
			return err
		}
		if err := os.Chmod(path, os.FileMode(file.Mode)&0o777); err != nil {
			return err
		}
	}
	return nil
}
