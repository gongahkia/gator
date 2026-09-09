package artifact

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// WriteArchive emits a deterministic tar.gz containing only the canonical
// manifest and verified output. Private workspace metadata and source paths
// are deliberately excluded.
func WriteArchive(destination io.Writer, bundle Bundle) error {
	if destination == nil {
		return fmt.Errorf("artifact archive destination is required")
	}
	if err := VerifyBundle(bundle); err != nil {
		return fmt.Errorf("verify artifact bundle before export: %w", err)
	}
	manifest, err := json.MarshalIndent(bundle.Manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode artifact manifest: %w", err)
	}
	manifest = append(manifest, '\n')

	gzipWriter := gzip.NewWriter(destination)
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	closeWriters := func() error {
		if err := tarWriter.Close(); err != nil {
			_ = gzipWriter.Close()
			return fmt.Errorf("close artifact tar archive: %w", err)
		}
		if err := gzipWriter.Close(); err != nil {
			return fmt.Errorf("close artifact gzip archive: %w", err)
		}
		return nil
	}
	if err := writeTarFile(tarWriter, "manifest.json", manifest); err != nil {
		_ = closeWriters()
		return err
	}
	for _, directory := range archiveDirectories(bundle.Manifest.Artifacts) {
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: directory + "/", Typeflag: tar.TypeDir, Mode: 0o700,
			ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(),
		}); err != nil {
			_ = closeWriters()
			return fmt.Errorf("write artifact archive directory %q: %w", directory, err)
		}
	}
	for _, file := range bundle.Manifest.Artifacts {
		contents, err := bundle.Output.ReadRegularFile(filepath.FromSlash(file.Path), bundle.Manifest.Contract.MaxArtifactBytes)
		if err != nil {
			_ = closeWriters()
			return fmt.Errorf("read artifact %q for export: %w", file.Path, err)
		}
		if err := writeTarFile(tarWriter, path.Join("output", file.Path), contents); err != nil {
			_ = closeWriters()
			return err
		}
	}
	for _, source := range bundle.Manifest.ConnectedSources {
		if source.SnapshotPath == "" {
			continue
		}
		contents, err := bundle.Root.ReadRegularFile(filepath.FromSlash(source.SnapshotPath), 256*1024)
		if err != nil {
			_ = closeWriters()
			return err
		}
		if err := writeTarFile(tarWriter, source.SnapshotPath, contents); err != nil {
			_ = closeWriters()
			return err
		}
	}
	for _, source := range bundle.Manifest.Evidence {
		if source.SnapshotPath == "" {
			continue
		}
		data, err := bundle.Root.ReadRegularFile(source.SnapshotPath, 512*1024)
		if err != nil {
			_ = closeWriters()
			return err
		}
		if err := writeTarFile(tarWriter, source.SnapshotPath, data); err != nil {
			_ = closeWriters()
			return err
		}
	}
	return closeWriters()
}

func writeTarFile(writer *tar.Writer, name string, contents []byte) error {
	header := &tar.Header{
		Name: name, Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(contents)),
		ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(),
	}
	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("write artifact archive header %q: %w", name, err)
	}
	if _, err := writer.Write(contents); err != nil {
		return fmt.Errorf("write artifact archive file %q: %w", name, err)
	}
	return nil
}

func archiveDirectories(files []File) []string {
	directories := map[string]struct{}{"output": {}}
	for _, file := range files {
		current := path.Dir(path.Join("output", file.Path))
		for current != "." && current != "/" {
			directories[current] = struct{}{}
			current = path.Dir(current)
		}
	}
	result := make([]string, 0, len(directories))
	for directory := range directories {
		result = append(result, strings.TrimSuffix(directory, "/"))
	}
	sort.Slice(result, func(left, right int) bool {
		leftDepth := strings.Count(result[left], "/")
		rightDepth := strings.Count(result[right], "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return result[left] < result[right]
	})
	return result
}
