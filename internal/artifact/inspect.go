package artifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/workspace"
)

const (
	maxObservedArtifacts = 256
	maxDiagnosticBytes   = 512
)

// Inspection is a trusted projection of one output root against an outcome
// contract. Validation failures are represented as results rather than Go
// errors so callers can persist useful evidence for an unsuccessful run.
type Inspection struct {
	Files       []File
	Validations []ValidationResult
	Passed      bool
}

// Inspect walks the complete bounded output root, records regular artifacts,
// and evaluates every required artifact against contract. Symlinks and special
// files fail closed because a sealed artifact bundle must be self-contained.
func Inspect(root workspace.Root, contract Contract) (Inspection, error) {
	contract = contract.Normalize()
	if err := contract.Validate(); err != nil {
		return Inspection{}, err
	}
	files, contents, err := inspectFiles(root, contract)
	if err != nil {
		return Inspection{}, err
	}
	result := Inspection{Files: files, Passed: true}
	for _, requirement := range contract.Artifacts {
		file, exists := fileNamed(files, requirement.Path)
		data := contents[requirement.Path]
		result.Validations = append(result.Validations, validationResult(
			requirement.Path, ArtifactExists, exists, "required artifact does not exist",
		))
		if !exists {
			result.Passed = false
			for _, validation := range requirement.Validations {
				if validation.Kind == ArtifactExists {
					continue
				}
				result.Validations = append(result.Validations, validationResult(
					requirement.Path, validation.Kind, false, "artifact is unavailable",
				))
			}
			if len(requirement.MediaTypes) > 0 {
				result.Validations = append(result.Validations, validationResult(
					requirement.Path, MediaType, false, "artifact is unavailable",
				))
			}
			continue
		}

		if len(requirement.MediaTypes) > 0 {
			allowed := containsString(requirement.MediaTypes, file.MediaType)
			diagnostic := ""
			if !allowed {
				diagnostic = fmt.Sprintf("detected %s; allowed: %s", file.MediaType, strings.Join(requirement.MediaTypes, ", "))
				result.Passed = false
			}
			result.Validations = append(result.Validations, validationResult(requirement.Path, MediaType, allowed, diagnostic))
		}
		for _, validation := range requirement.Validations {
			if validation.Kind == ArtifactExists {
				continue
			}
			passed, diagnostic := validateContents(validation, data)
			if !passed {
				result.Passed = false
			}
			result.Validations = append(result.Validations, validationResult(requirement.Path, validation.Kind, passed, diagnostic))
		}
	}
	return result, nil
}

func inspectFiles(root workspace.Root, contract Contract) ([]File, map[string][]byte, error) {
	var paths []string
	err := filepath.WalkDir(root.Path(), func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root.Path() {
			return nil
		}
		relative, err := filepath.Rel(root.Path(), current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact output %q is a symlink", relative)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("artifact output %q is not a regular file", relative)
		}
		if len(paths) == maxObservedArtifacts {
			return fmt.Errorf("artifact output contains more than %d files", maxObservedArtifacts)
		}
		paths = append(paths, relative)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("inspect artifact output: %w", err)
	}
	sort.Strings(paths)

	required := make(map[string]struct{}, len(contract.Artifacts))
	for _, requirement := range contract.Artifacts {
		required[requirement.Path] = struct{}{}
	}
	files := make([]File, 0, len(paths))
	contents := make(map[string][]byte, len(required))
	var total int64
	for _, relative := range paths {
		data, err := root.ReadRegularFile(filepath.FromSlash(relative), contract.MaxArtifactBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("read artifact %q: %w", relative, err)
		}
		total += int64(len(data))
		if total > contract.MaxTotalBytes {
			return nil, nil, fmt.Errorf("artifact output exceeds the %d-byte total limit", contract.MaxTotalBytes)
		}
		digest := sha256.Sum256(data)
		files = append(files, File{
			Path:      relative,
			MediaType: detectMediaType(relative, data),
			Bytes:     int64(len(data)),
			SHA256:    hex.EncodeToString(digest[:]),
		})
		if _, keep := required[relative]; keep {
			contents[relative] = data
		}
	}
	return files, contents, nil
}

func detectMediaType(path string, data []byte) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return "text/markdown"
	case ".csv":
		return "text/csv"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".pdf":
		return "application/pdf"
	}
	if value := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); value != "" {
		mediaType, _, err := mime.ParseMediaType(value)
		if err == nil {
			return mediaType
		}
	}
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	return http.DetectContentType(sample)
}

func validateContents(validation Validation, data []byte) (bool, string) {
	switch validation.Kind {
	case NonEmpty:
		if len(data) == 0 {
			return false, "artifact is empty"
		}
		return true, ""
	case UTF8:
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return false, "artifact is not NUL-free UTF-8 text"
		}
		return true, ""
	case JSON:
		decoder := json.NewDecoder(bytes.NewReader(data))
		var value any
		if err := decoder.Decode(&value); err != nil {
			return false, boundedDiagnostic("decode JSON: " + err.Error())
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			if err == nil {
				return false, "JSON artifact contains more than one value"
			}
			return false, boundedDiagnostic("decode JSON: " + err.Error())
		}
		return true, ""
	case CSV:
		reader := csv.NewReader(bytes.NewReader(data))
		rows, err := reader.ReadAll()
		if err != nil {
			return false, boundedDiagnostic("decode CSV: " + err.Error())
		}
		if len(rows) == 0 || len(rows[0]) == 0 {
			return false, "CSV artifact has no header"
		}
		seen := make(map[string]struct{}, len(rows[0]))
		for _, heading := range rows[0] {
			heading = strings.TrimSpace(heading)
			if heading == "" {
				return false, "CSV artifact has an empty header field"
			}
			if _, duplicate := seen[heading]; duplicate {
				return false, fmt.Sprintf("CSV artifact repeats header %q", heading)
			}
			seen[heading] = struct{}{}
		}
		return true, ""
	case Contains:
		if !bytes.Contains(data, []byte(validation.Value)) {
			return false, "artifact does not contain the required marker"
		}
		return true, ""
	default:
		return false, fmt.Sprintf("validator %q is unavailable", validation.Kind)
	}
}

func validationResult(path string, kind ValidationKind, passed bool, diagnostic string) ValidationResult {
	if passed {
		diagnostic = ""
	}
	return ValidationResult{Path: path, Kind: kind, Passed: passed, Diagnostic: boundedDiagnostic(diagnostic)}
}

func boundedDiagnostic(value string) string {
	if len(value) <= maxDiagnosticBytes {
		return value
	}
	return value[:maxDiagnosticBytes]
}

func fileNamed(files []File, path string) (File, bool) {
	index := sort.Search(len(files), func(index int) bool { return files[index].Path >= path })
	if index < len(files) && files[index].Path == path {
		return files[index], true
	}
	return File{}, false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
