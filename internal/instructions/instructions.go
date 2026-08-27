// Package instructions resolves repository guidance without making project
// files executable. Guidance is prompt context, while hooks, MCP servers, and
// extensions remain separately trusted capabilities.
package instructions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/workspace"
)

const (
	maxFileBytes     = 64 * 1024
	maxCombinedBytes = 256 * 1024
	rulesPath        = ".gator/rules.json"
)

// Set is the resolved, ordered guidance supplied to one run.
type Set struct {
	Content string
	Files   []string
	Policy  ProfilePolicy
}

type rulesDocument struct {
	Version int    `json:"version"`
	Rules   []rule `json:"rules"`
}

type rule struct {
	ID           string   `json:"id"`
	Paths        []string `json:"paths"`
	Instructions string   `json:"instructions,omitempty"`
	File         string   `json:"file,omitempty"`
}

type profilesDocument struct {
	Version  int       `json:"version"`
	Profiles []profile `json:"profiles"`
	Roles    []role    `json:"roles"`
}

type profile struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	Instructions string        `json:"instructions,omitempty"`
	File         string        `json:"file,omitempty"`
	Policy       ProfilePolicy `json:"policy,omitempty"`
}

type role struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Kind         string        `json:"kind"`
	Instructions string        `json:"instructions,omitempty"`
	File         string        `json:"file,omitempty"`
	Policy       ProfilePolicy `json:"policy,omitempty"`
}

// Role is a validated project-defined specialization. Policy is a restrictive
// meet applied after the parent profile: a role can remove tools, force strict
// sandboxing or denied network, and reduce the step budget, but never widen
// authority.
type Role struct {
	Name         string
	Description  string
	Kind         string
	Instructions string
	Policy       ProfilePolicy
}

const (
	RoleReadOnly = "readonly"
	RoleWriter   = "writer"
)

// Load collects root guidance, directory-scoped AGENTS files for the requested
// paths, and matching declarative .gator/rules.json rules. Earlier layers are
// shown first; more-specific layers therefore appear later and can refine them.
func Load(repository string, scopes []string) (Set, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return Set{}, err
	}
	normalized, err := normalizeScopes(scopes)
	if err != nil {
		return Set{}, err
	}
	var files []string
	var sections []string
	appendFile := func(relative string) error {
		for _, existing := range files {
			if existing == relative {
				return nil
			}
		}
		contents, err := root.ReadRegularFile(relative, maxFileBytes)
		if err != nil {
			if isMissingFileError(err) {
				return nil
			}
			return fmt.Errorf("read project instructions %q: %w", relative, err)
		}
		if len(strings.TrimSpace(string(contents))) == 0 {
			files = append(files, relative)
			return nil
		}
		if totalLength(sections)+len(contents) > maxCombinedBytes {
			return errors.New("project instructions exceed the 256 KiB combined limit")
		}
		files = append(files, relative)
		sections = append(sections, "Instructions from "+relative+":\n"+strings.TrimSpace(string(contents)))
		return nil
	}

	// An override replaces the root AGENTS.md layer, mirroring the established
	// convention without making a project file executable.
	if exists, err := regularFileExists(root, "AGENTS.override.md"); err != nil {
		return Set{}, err
	} else if exists {
		if err := appendFile("AGENTS.override.md"); err != nil {
			return Set{}, err
		}
	} else if err := appendFile("AGENTS.md"); err != nil {
		return Set{}, err
	}
	if err := appendFile(".gator/AGENTS.md"); err != nil {
		return Set{}, err
	}
	for _, scope := range normalized {
		for _, directory := range ancestorDirectories(scope) {
			if directory == "." {
				continue
			}
			override := filepath.ToSlash(filepath.Join(directory, "AGENTS.override.md"))
			exists, err := regularFileExists(root, override)
			if err != nil {
				return Set{}, err
			}
			if exists {
				if err := appendFile(override); err != nil {
					return Set{}, err
				}
				continue
			}
			if err := appendFile(filepath.ToSlash(filepath.Join(directory, "AGENTS.md"))); err != nil {
				return Set{}, err
			}
		}
	}
	ruleSections, ruleFiles, err := loadRules(root, normalized)
	if err != nil {
		return Set{}, err
	}
	if totalLength(sections)+totalLength(ruleSections) > maxCombinedBytes {
		return Set{}, errors.New("project instructions exceed the 256 KiB combined limit")
	}
	sections = append(sections, ruleSections...)
	files = append(files, ruleFiles...)
	return Set{Content: strings.Join(sections, "\n\n"), Files: files}, nil
}

// LoadWithProfile adds one explicitly named, non-executable agent profile
// after ordinary scoped rules. Profiles are selected by the developer rather
// than being implicitly activated by repository content.
func LoadWithProfile(repository string, scopes []string, name string) (Set, error) {
	set, err := Load(repository, scopes)
	if err != nil || strings.TrimSpace(name) == "" {
		return set, err
	}
	name = strings.TrimSpace(name)
	if !validProfileName(name) {
		return Set{}, fmt.Errorf("invalid agent profile %q", name)
	}
	root, err := workspace.Open(repository)
	if err != nil {
		return Set{}, err
	}
	document, err := loadProfilesDocument(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Set{}, fmt.Errorf("agent profile %q is not configured", name)
		}
		return Set{}, err
	}
	if err := validateProfilesDocument(document); err != nil {
		return Set{}, err
	}
	for _, candidate := range document.Profiles {
		if candidate.Name != name {
			continue
		}
		body, profileFile, err := loadAgentInstructions(root, candidate.Name, candidate.Instructions, candidate.File, "profile")
		if err != nil {
			return Set{}, err
		}
		files := append([]string(nil), set.Files...)
		files = append(files, ".gator/agents.json")
		if profileFile != "" {
			files = append(files, profileFile)
		}
		section := "Instructions from selected agent profile " + candidate.Name + ":\n" + body
		if len(set.Content)+len(section) > maxCombinedBytes {
			return Set{}, errors.New("project instructions exceed the 256 KiB combined limit")
		}
		if strings.TrimSpace(set.Content) != "" {
			set.Content += "\n\n"
		}
		set.Content += section
		set.Files = files
		set.Policy = candidate.Policy
		set.Policy.Omit = mergeOmit(candidate.Policy.Omit)
		return set, nil
	}
	return Set{}, fmt.Errorf("agent profile %q is not configured", name)
}

// ListProfiles returns developer-selectable profiles without activating any.
func ListProfiles(repository string) ([]Profile, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return nil, err
	}
	document, err := loadProfilesDocument(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if err := validateProfilesDocument(document); err != nil {
		return nil, err
	}
	result := make([]Profile, 0, len(document.Profiles))
	for _, candidate := range document.Profiles {
		policy := candidate.Policy
		policy.Omit = mergeOmit(policy.Omit)
		result = append(result, Profile{Name: candidate.Name, Description: candidate.Description, Policy: policy})
	}
	return result, nil
}

// LoadRoles loads validated project-defined subagent specializations. Roles
// are non-executable prompt data and remain constrained by their fixed Kind.
// A missing role file is normal for projects which only use profiles.
func LoadRoles(repository string) ([]Role, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return nil, err
	}
	document, err := loadProfilesDocument(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if err := validateProfilesDocument(document); err != nil {
		return nil, err
	}
	result := make([]Role, 0, len(document.Roles))
	for _, candidate := range document.Roles {
		body, _, err := loadAgentInstructions(root, candidate.Name, candidate.Instructions, candidate.File, "role")
		if err != nil {
			return nil, err
		}
		if body == "" {
			return nil, fmt.Errorf("agent role %q has empty instructions", candidate.Name)
		}
		result = append(result, Role{Name: candidate.Name, Description: candidate.Description, Kind: candidate.Kind, Instructions: body, Policy: candidate.Policy})
	}
	return result, nil
}

func loadProfilesDocument(root workspace.Root) (profilesDocument, error) {
	contents, err := root.ReadRegularFile(".gator/agents.json", maxFileBytes)
	if err != nil {
		if isMissingFileError(err) {
			return profilesDocument{}, fs.ErrNotExist
		}
		return profilesDocument{}, fmt.Errorf("read agent profiles: %w", err)
	}
	var document profilesDocument
	decoder := json.NewDecoder(strings.NewReader(string(contents)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return profilesDocument{}, fmt.Errorf("decode agent profiles: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return profilesDocument{}, fmt.Errorf("decode agent profiles: %w", err)
	}
	return document, nil
}

func validateProfilesDocument(document profilesDocument) error {
	if document.Version != 1 || len(document.Profiles) > 64 || len(document.Roles) > 32 {
		return errors.New("agent definitions have an unsupported version or too many entries")
	}
	profiles := make(map[string]struct{}, len(document.Profiles))
	for _, candidate := range document.Profiles {
		if !validProfileName(candidate.Name) {
			return fmt.Errorf("invalid agent profile %q", candidate.Name)
		}
		if _, duplicate := profiles[candidate.Name]; duplicate {
			return fmt.Errorf("agent profile %q is repeated", candidate.Name)
		}
		profiles[candidate.Name] = struct{}{}
		if err := validateAgentInstructions(candidate.Name, candidate.Instructions, candidate.File, "profile"); err != nil {
			return err
		}
		if err := validateProfilePolicy(candidate.Name, candidate.Policy); err != nil {
			return err
		}
		candidate.Policy.Omit = mergeOmit(candidate.Policy.Omit)
	}
	roles := make(map[string]struct{}, len(document.Roles))
	for _, candidate := range document.Roles {
		if !validProfileName(candidate.Name) {
			return fmt.Errorf("invalid agent role %q", candidate.Name)
		}
		if _, duplicate := roles[candidate.Name]; duplicate {
			return fmt.Errorf("agent role %q is repeated", candidate.Name)
		}
		roles[candidate.Name] = struct{}{}
		if candidate.Kind != RoleReadOnly && candidate.Kind != RoleWriter {
			return fmt.Errorf("agent role %q has unsupported kind %q", candidate.Name, candidate.Kind)
		}
		if description := strings.TrimSpace(candidate.Description); description == "" || len(description) > 512 || strings.ContainsAny(description, "\r\n") {
			return fmt.Errorf("agent role %q requires a one-line description no longer than 512 bytes", candidate.Name)
		}
		if err := validateAgentInstructions(candidate.Name, candidate.Instructions, candidate.File, "role"); err != nil {
			return err
		}
		if err := validateProfilePolicy("role "+candidate.Name, candidate.Policy); err != nil {
			return err
		}
	}
	return nil
}

func validateAgentInstructions(name, inline, file, kind string) error {
	if (strings.TrimSpace(inline) == "") == (strings.TrimSpace(file) == "") {
		return fmt.Errorf("agent %s %q requires exactly one of instructions or file", kind, name)
	}
	return nil
}

func loadAgentInstructions(root workspace.Root, name, inline, file, kind string) (string, string, error) {
	body := strings.TrimSpace(inline)
	if file == "" {
		return body, "", nil
	}
	path := filepath.ToSlash(strings.TrimSpace(file))
	path = pathpkgClean(path)
	if !strings.HasPrefix(path, ".gator/") {
		return "", "", fmt.Errorf("agent %s %q file must stay below .gator", kind, name)
	}
	contents, err := root.ReadRegularFile(filepath.FromSlash(path), maxFileBytes)
	if err != nil {
		return "", "", fmt.Errorf("read agent %s %q: %w", kind, name, err)
	}
	return strings.TrimSpace(string(contents)), path, nil
}

func validProfileName(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || (index > 0 && (character == '-' || character == '_')) {
			continue
		}
		return false
	}
	return true
}

func pathpkgClean(value string) string { return path.Clean(value) }

func normalizeScopes(scopes []string) ([]string, error) {
	seen := make(map[string]struct{}, len(scopes))
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = filepath.ToSlash(strings.TrimSpace(scope))
		if scope == "" {
			continue
		}
		if strings.HasPrefix(scope, "/") || scope == ".." || strings.HasPrefix(scope, "../") {
			return nil, fmt.Errorf("instruction scope %q escapes the repository", scope)
		}
		clean := path.Clean(scope)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("instruction scope %q is invalid", scope)
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	sort.Strings(result)
	return result, nil
}

func ancestorDirectories(scope string) []string {
	directory := path.Dir(scope)
	if path.Ext(scope) == "" {
		directory = scope
	}
	var result []string
	for directory != "." && directory != "/" {
		result = append(result, directory)
		directory = path.Dir(directory)
	}
	for first, last := 0, len(result)-1; first < last; first, last = first+1, last-1 {
		result[first], result[last] = result[last], result[first]
	}
	return result
}

func loadRules(root workspace.Root, scopes []string) ([]string, []string, error) {
	if len(scopes) == 0 {
		return nil, nil, nil
	}
	contents, err := root.ReadRegularFile(rulesPath, maxFileBytes)
	if err != nil {
		if isMissingFileError(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read project rules: %w", err)
	}
	var document rulesDocument
	decoder := json.NewDecoder(strings.NewReader(string(contents)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, nil, fmt.Errorf("decode project rules: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return nil, nil, fmt.Errorf("decode project rules: %w", err)
	}
	if document.Version != 1 {
		return nil, nil, fmt.Errorf("unsupported project rules version %d", document.Version)
	}
	if len(document.Rules) > 128 {
		return nil, nil, errors.New("project rules contain too many entries")
	}
	var sections []string
	files := []string{rulesPath}
	for index, rule := range document.Rules {
		if len(rule.Paths) == 0 || len(rule.Paths) > 32 {
			return nil, nil, fmt.Errorf("project rule %d requires 1-32 paths", index+1)
		}
		if (strings.TrimSpace(rule.Instructions) == "") == (strings.TrimSpace(rule.File) == "") {
			return nil, nil, fmt.Errorf("project rule %d requires exactly one of instructions or file", index+1)
		}
		matched := false
		for _, pattern := range rule.Paths {
			if err := validateGlob(pattern); err != nil {
				return nil, nil, fmt.Errorf("project rule %d: %w", index+1, err)
			}
			for _, scope := range scopes {
				if globMatch(pattern, scope) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			continue
		}
		body := strings.TrimSpace(rule.Instructions)
		label := "project rule"
		if rule.ID != "" {
			label += " " + rule.ID
		}
		if rule.File != "" {
			file := filepath.ToSlash(strings.TrimSpace(rule.File))
			cleanFile := path.Clean(file)
			if !strings.HasPrefix(cleanFile, ".gator/") {
				return nil, nil, fmt.Errorf("project rule %d file must stay below .gator", index+1)
			}
			file = cleanFile
			contents, err := root.ReadRegularFile(file, maxFileBytes)
			if err != nil {
				return nil, nil, fmt.Errorf("read project rule %d: %w", index+1, err)
			}
			body = strings.TrimSpace(string(contents))
			files = append(files, file)
			label += " from " + file
		}
		if body != "" {
			sections = append(sections, "Instructions from "+label+":\n"+body)
		}
	}
	return sections, files, nil
}

func regularFileExists(root workspace.Root, relative string) (bool, error) {
	_, err := root.ReadRegularFile(relative, maxFileBytes)
	if err == nil {
		return true, nil
	}
	if isMissingFileError(err) {
		return false, nil
	}
	return false, err
}

func isMissingFileError(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

func requireEOF(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected second JSON value")
		}
		return err
	}
	return nil
}

func totalLength(sections []string) int {
	total := 0
	for _, section := range sections {
		total += len(section)
	}
	return total
}

func validateGlob(pattern string) error {
	if strings.TrimSpace(pattern) == "" || strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "../") {
		return fmt.Errorf("invalid path glob %q", pattern)
	}
	_, err := path.Match(strings.ReplaceAll(pattern, "**", "*"), "probe")
	if err != nil {
		return fmt.Errorf("invalid path glob %q: %w", pattern, err)
	}
	return nil
}

func globMatch(pattern, value string) bool {
	pattern = path.Clean(strings.TrimPrefix(pattern, "./"))
	value = path.Clean(value)
	patternParts := strings.Split(pattern, "/")
	valueParts := strings.Split(value, "/")
	var match func(int, int) bool
	match = func(patternIndex, valueIndex int) bool {
		if patternIndex == len(patternParts) {
			return valueIndex == len(valueParts)
		}
		if patternParts[patternIndex] == "**" {
			for index := valueIndex; index <= len(valueParts); index++ {
				if match(patternIndex+1, index) {
					return true
				}
			}
			return false
		}
		if valueIndex == len(valueParts) {
			return false
		}
		ok, err := path.Match(patternParts[patternIndex], valueParts[valueIndex])
		return err == nil && ok && match(patternIndex+1, valueIndex+1)
	}
	return match(0, 0)
}
