package lifecycle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/state"
)

type BackupManifest struct {
	Version   int
	CreatedAt time.Time
	StateDir  string
	Files     map[string]string
	Excluded  []string
}
type cancelReader struct {
	ctx context.Context
	io.Reader
}

func (r cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

// Export makes a private, checksummed backup of retained Work content. Runtime
// capabilities, credentials, provider configuration and browser profiles are
// excluded. Source snapshots and transcripts can still contain private data.
func (s Store) Export(ctx context.Context, destination string) (BackupManifest, error) {
	manifest := BackupManifest{Version: 1, CreatedAt: time.Now().UTC(), StateDir: s.stateDir, Files: map[string]string{}, Excluded: []string{"credentials and provider configuration", "browser/desktop profiles", "runtime access tokens and leases", "rebuildable retrieval and conversation indexes", "shared usage counters", "original external project directories"}}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return manifest, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return manifest, err
	}
	absolute = filepath.Join(parent, filepath.Base(absolute))
	rootPath, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return manifest, err
	}
	rel, err := filepath.Rel(rootPath, absolute)
	if err != nil || rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
		return manifest, errors.New("place the backup outside the private Gator data directory")
	}
	if _, err = os.Lstat(absolute); err == nil {
		return manifest, errors.New("backup destination already exists; choose a new path")
	} else if !errors.Is(err, os.ErrNotExist) {
		return manifest, err
	}
	release, err := state.Guard(s.stateDir, true)
	if err != nil {
		return manifest, err
	}
	defer release()
	if err = s.ensureIdle(); err != nil {
		return manifest, err
	}
	paths, _ := roots("all")
	items, err := s.walk(ctx, paths, false)
	if err != nil {
		return manifest, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return manifest, err
	}
	defer root.Close()
	f, err := os.CreateTemp(parent, ".gator-backup-*")
	if err != nil {
		return manifest, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return manifest, err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	var total int64
	for _, item := range items {
		if err = ctx.Err(); err != nil {
			return manifest, err
		}
		if item.Directory || !backupContent("gator/"+filepath.ToSlash(item.Path)) {
			continue
		}
		if item.Bytes > 1<<30 {
			return manifest, errors.New("backup file exceeds the 1 GiB verification bound")
		}
		total += item.Bytes
		if total > 10<<30 {
			return manifest, errors.New("backup exceeds the 10 GiB verification bound")
		}
		file, openErr := root.Open(item.Path)
		if openErr != nil {
			return manifest, openErr
		}
		header := &tar.Header{Name: "gator/" + filepath.ToSlash(item.Path), Mode: 0600, Size: item.Bytes, ModTime: time.Unix(0, item.Modified), Typeflag: tar.TypeReg}
		if err = tw.WriteHeader(header); err != nil {
			file.Close()
			return manifest, err
		}
		digest := sha256.New()
		_, err = io.Copy(io.MultiWriter(tw, digest), cancelReader{ctx, file})
		info, statErr := file.Stat()
		file.Close()
		if err != nil {
			return manifest, err
		}
		if statErr != nil || info.Size() != item.Bytes || info.ModTime().UnixNano() != item.Modified {
			return manifest, errors.New("private data changed during export; retry the backup")
		}
		manifest.Files[header.Name] = hex.EncodeToString(digest.Sum(nil))
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return manifest, err
	}
	if len(data) > 32<<20 || total+int64(len(data)) > 10<<30 {
		return manifest, errors.New("backup manifest exceeds its verification bound")
	}
	if err = tw.WriteHeader(&tar.Header{Name: "gator-backup.json", Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		return manifest, err
	}
	if _, err = tw.Write(data); err != nil {
		return manifest, err
	}
	if err = tw.Close(); err != nil {
		return manifest, err
	}
	if err = gz.Close(); err != nil {
		return manifest, err
	}
	if err = f.Sync(); err != nil {
		return manifest, err
	}
	if err = f.Close(); err != nil {
		return manifest, err
	}
	// Link is exclusive: an independently created destination is never replaced.
	if err = os.Link(f.Name(), absolute); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func VerifyBackup(ctx context.Context, filename string) (BackupManifest, error) {
	return readBackup(ctx, filename, nil)
}

// readBackup has one parser for verification and extraction; the sink sees only
// regular allowlisted files and must consume each entry completely.
func readBackup(ctx context.Context, filename string, sink func(*tar.Header, io.Reader) error) (BackupManifest, error) {
	var manifest BackupManifest
	f, err := os.Open(filename)
	if err != nil {
		return manifest, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return manifest, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	actual := map[string]string{}
	var total int64
	seenManifest := false
	for {
		if err = ctx.Err(); err != nil {
			return manifest, err
		}
		header, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return manifest, nextErr
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > 1<<30 || path.Clean(header.Name) != header.Name || !filepath.IsLocal(header.Name) || strings.Count(header.Name, "/") > 32 || strings.ContainsAny(header.Name, "\\\x00\r\n") {
			return manifest, errors.New("backup contains an unsafe entry")
		}
		total += header.Size
		if total > 10<<30 {
			return manifest, errors.New("backup exceeds the 10 GiB verification bound")
		}
		if header.Name == "gator-backup.json" {
			if seenManifest || header.Size > 32<<20 {
				return manifest, errors.New("invalid backup manifest")
			}
			seenManifest = true
			data, err := io.ReadAll(io.LimitReader(tr, 32<<20))
			if err != nil {
				return manifest, err
			}
			if err = json.Unmarshal(data, &manifest); err != nil {
				return manifest, err
			}
			continue
		}
		if !backupContent(header.Name) {
			return manifest, errors.New("backup entry escapes private content")
		}
		if _, found := actual[header.Name]; found {
			return manifest, errors.New("duplicate backup entry")
		}
		digest := sha256.New()
		reader := io.TeeReader(cancelReader{ctx, tr}, digest)
		if sink == nil {
			_, err = io.Copy(io.Discard, reader)
		} else {
			err = sink(header, reader)
		}
		if err != nil {
			return manifest, err
		}
		actual[header.Name] = hex.EncodeToString(digest.Sum(nil))
		if len(actual) > 500000 {
			return manifest, errors.New("backup contains too many files")
		}
	}
	remaining := int64(10<<30) - total + 1
	drained, drainErr := io.Copy(io.Discard, io.LimitReader(cancelReader{ctx, gz}, remaining))
	if drainErr != nil {
		return manifest, drainErr
	}
	if drained >= remaining {
		return manifest, errors.New("backup exceeds its verification bound")
	}
	if !seenManifest || manifest.Version != 1 || len(actual) != len(manifest.Files) {
		return manifest, errors.New("backup manifest does not match its content")
	}
	for name, digest := range actual {
		if manifest.Files[name] != digest {
			return manifest, fmt.Errorf("backup checksum mismatch for %s", name)
		}
	}
	return manifest, nil
}

func backupContent(name string) bool {
	if !strings.HasPrefix(name, "gator/") {
		return false
	}
	rel := strings.TrimPrefix(name, "gator/")
	if rel == "memory/.write-lock" {
		return false
	}
	if strings.HasPrefix(rel, "work-control/") || strings.HasPrefix(rel, "retrieval/") || strings.HasPrefix(rel, "cache/") {
		return false
	}
	if (strings.HasPrefix(rel, "work-history/") || strings.HasPrefix(rel, "goals/plans/")) && strings.HasSuffix(rel, ".lock") {
		return false
	}
	if strings.HasPrefix(rel, "external-actions/") && (strings.HasSuffix(rel, ".lock") || strings.HasPrefix(path.Base(rel), ".gator-write-")) {
		return false
	}
	if strings.HasPrefix(rel, "files/") && (strings.HasSuffix(rel, ".lock") || strings.HasSuffix(rel, ".tmp")) {
		return false
	}
	roots, _ := roots("all")
	for _, root := range roots {
		root = filepath.ToSlash(root)
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}
