package skill

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/store"
)

const (
	maxSkillFiles      = 128
	maxSkillBytes      = 10 << 20
	maxFileBytes       = 1 << 20
	maxSkillsPerImport = 64
	maxCatalogueRepos  = 32
)

var githubRepositoryURL = regexp.MustCompile(`https://github\.com/[^\s\)\]\}>"']+`)

type ImportInput struct {
	SourceType        string `json:"source_type"`
	SourceURI         string `json:"source_uri"`
	SourceRef         string `json:"source_ref"`
	CredentialEnv     string `json:"credential_env"`
	Mode              string `json:"mode"`
	FollowReadmeLinks bool   `json:"follow_readme_links"`
}

type RejectedSkill struct {
	SourceURI string `json:"source_uri,omitempty"`
	Path      string `json:"path"`
	Error     string `json:"error"`
}

type CatalogueStats struct {
	RepositoriesDiscovered int `json:"repositories_discovered"`
	RepositoriesScanned    int `json:"repositories_scanned"`
	RepositoriesSkipped    int `json:"repositories_skipped"`
}

type ImportResult struct {
	Imports   []domain.SkillImport  `json:"imports"`
	Packages  []domain.SkillPackage `json:"packages"`
	Rejected  []RejectedSkill       `json:"rejected"`
	Catalogue *CatalogueStats       `json:"catalogue,omitempty"`
}

type discoveredSkill struct {
	Root string
	Path string
}

type sourceProvenance struct {
	ResolvedRef string
	TreeDigest  string
}

type Service struct {
	store        *store.Store
	artifactsDir string
	httpClient   *http.Client
}

func New(st *store.Store, artifactsDir string) *Service {
	return &Service{store: st, artifactsDir: filepath.Join(artifactsDir, "skills"), httpClient: &http.Client{Timeout: 45 * time.Second}}
}

func (s *Service) Import(ctx context.Context, input ImportInput) (domain.SkillImport, domain.SkillPackage, error) {
	result, err := s.ImportAll(ctx, input)
	if err != nil {
		return domain.SkillImport{}, domain.SkillPackage{}, err
	}
	return result.Imports[0], result.Packages[0], nil
}

func (s *Service) ImportAll(ctx context.Context, input ImportInput) (ImportResult, error) {
	input.SourceType, input.SourceURI, input.SourceRef, input.CredentialEnv, input.Mode = strings.TrimSpace(input.SourceType), strings.TrimSpace(input.SourceURI), strings.TrimSpace(input.SourceRef), strings.TrimSpace(input.CredentialEnv), strings.ToLower(strings.TrimSpace(input.Mode))
	if input.SourceType != "git" && input.SourceType != "oci" {
		return ImportResult{}, fmt.Errorf("source_type must be git or oci")
	}
	if input.SourceURI == "" {
		return ImportResult{}, fmt.Errorf("source_uri is required")
	}
	if input.CredentialEnv != "" && !validEnvName(input.CredentialEnv) {
		return ImportResult{}, fmt.Errorf("credential_env is invalid")
	}
	if input.Mode == "" {
		input.Mode = "auto"
	}
	if input.Mode != "auto" && input.Mode != "native" && input.Mode != "adapted" {
		return ImportResult{}, fmt.Errorf("mode must be auto, native, or adapted")
	}
	temporary, err := os.MkdirTemp("", "norbot-skill-")
	if err != nil {
		return ImportResult{}, err
	}
	defer os.RemoveAll(temporary)
	root := filepath.Join(temporary, "bundle")
	var provenance sourceProvenance
	switch input.SourceType {
	case "git":
		provenance, err = materializeGit(ctx, root, input)
	case "oci":
		provenance, err = s.materializeOCI(ctx, root, input)
	}
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{}
	if err := s.importSource(ctx, root, input, provenance, &result); err != nil {
		return ImportResult{}, err
	}
	if input.FollowReadmeLinks {
		repositories, stats, err := discoverReadmeRepositories(root, input.SourceURI)
		if err != nil {
			return ImportResult{}, err
		}
		result.Catalogue = &stats
		for index, repository := range repositories {
			if len(result.Imports)+len(result.Rejected) >= maxSkillsPerImport {
				stats.RepositoriesSkipped += len(repositories) - index
				break
			}
			stats.RepositoriesScanned++
			linkedRoot := filepath.Join(temporary, "catalogue", fmt.Sprintf("%03d", index))
			linkedInput := input
			linkedInput.SourceType, linkedInput.SourceURI, linkedInput.SourceRef, linkedInput.CredentialEnv, linkedInput.FollowReadmeLinks = "git", repository, "", "", false
			linkedProvenance, err := materializeGit(ctx, linkedRoot, linkedInput)
			if err != nil {
				result.Rejected = append(result.Rejected, RejectedSkill{SourceURI: repository, Error: err.Error()})
				continue
			}
			before := len(result.Imports) + len(result.Rejected)
			if err := s.importSource(ctx, linkedRoot, linkedInput, linkedProvenance, &result); err != nil {
				result.Rejected = append(result.Rejected, RejectedSkill{SourceURI: repository, Error: err.Error()})
				continue
			}
			if len(result.Imports)+len(result.Rejected) == before {
				result.Rejected = append(result.Rejected, RejectedSkill{SourceURI: repository, Error: "no SKILL.md files found"})
			}
		}
	}
	if len(result.Imports) == 0 {
		if len(result.Rejected) > 0 {
			return ImportResult{}, fmt.Errorf("no safe skill bundles found: %s", result.Rejected[0].Error)
		}
		if result.Catalogue != nil && result.Catalogue.RepositoriesDiscovered > 0 {
			return ImportResult{}, fmt.Errorf("no SKILL.md files found in the source or its README-linked repositories")
		}
		return ImportResult{}, fmt.Errorf("no SKILL.md files found")
	}
	return result, nil
}

func (s *Service) importSource(ctx context.Context, root string, input ImportInput, provenance sourceProvenance, result *ImportResult) error {
	candidates, err := discoverSkills(root)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if len(result.Imports)+len(result.Rejected) >= maxSkillsPerImport {
			return nil
		}
		pkg, findings, mode, err := inspectCandidate(candidate, input)
		if err != nil {
			result.Rejected = append(result.Rejected, RejectedSkill{SourceURI: input.SourceURI, Path: candidate.Path, Error: err.Error()})
			continue
		}
		destination := filepath.Join(s.artifactsDir, strings.TrimPrefix(pkg.Digest, "sha256:"))
		if err := copyBundle(candidate.Root, destination); err != nil {
			return err
		}
		pkg.Path = destination
		pkg, err = s.store.UpsertSkillPackage(ctx, pkg)
		if err != nil {
			return err
		}
		findings["resolved_ref"] = provenance.ResolvedRef
		findings["tree_digest"] = provenance.TreeDigest
		imported, err := s.store.CreateSkillImport(ctx, domain.SkillImport{SourceType: input.SourceType, SourceURI: input.SourceURI, SourceRef: input.SourceRef, CredentialEnv: input.CredentialEnv, BundlePath: candidate.Path, Mode: mode, Digest: pkg.Digest, State: "scanned", ResolvedRef: provenance.ResolvedRef, TreeDigest: provenance.TreeDigest, TrustLevel: "untrusted", Findings: findings})
		if err != nil {
			return err
		}
		result.Imports = append(result.Imports, imported)
		result.Packages = append(result.Packages, pkg)
	}
	return nil
}

func (s *Service) Activate(ctx context.Context, id int64, reviewer string) (domain.SkillImport, error) {
	return s.store.ActivateSkillImport(ctx, id, reviewer)
}

func (s *Service) Imports(ctx context.Context) ([]domain.SkillImport, error) {
	return s.store.SkillImports(ctx)
}

func materializeGit(ctx context.Context, root string, input ImportInput) (sourceProvenance, error) {
	if !strings.HasPrefix(input.SourceURI, "https://") {
		return sourceProvenance{}, fmt.Errorf("git skill source must use https")
	}
	args := []string{"clone", "--depth", "1", input.SourceURI, root}
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if input.CredentialEnv != "" {
		secret := os.Getenv(input.CredentialEnv)
		if secret == "" {
			return sourceProvenance{}, fmt.Errorf("credential env %s is not set", input.CredentialEnv)
		}
		command.Env = append(command.Env, "GIT_HTTP_EXTRAHEADER=Authorization: Basic "+base64.StdEncoding.EncodeToString([]byte(secret)))
	}
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return sourceProvenance{}, fmt.Errorf("clone skill source: %s", detail)
	}
	if input.SourceRef != "" {
		if strings.HasPrefix(input.SourceRef, "-") || len(input.SourceRef) > 256 {
			return sourceProvenance{}, fmt.Errorf("source_ref is invalid")
		}
		fetch := exec.CommandContext(ctx, "git", "-C", root, "fetch", "--depth", "1", "origin", input.SourceRef)
		fetch.Env = command.Env
		if output, err := fetch.CombinedOutput(); err != nil {
			return sourceProvenance{}, fmt.Errorf("resolve skill source_ref: %s", strings.TrimSpace(string(output)))
		}
		checkout := exec.CommandContext(ctx, "git", "-C", root, "checkout", "--detach", "FETCH_HEAD")
		checkout.Env = command.Env
		if output, err := checkout.CombinedOutput(); err != nil {
			return sourceProvenance{}, fmt.Errorf("checkout skill source_ref: %s", strings.TrimSpace(string(output)))
		}
	}
	resolved, err := gitMetadata(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return sourceProvenance{}, err
	}
	tree, err := gitMetadata(ctx, root, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return sourceProvenance{}, err
	}
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		return sourceProvenance{}, err
	}
	format, err := gitMetadata(ctx, root, "rev-parse", "--show-object-format")
	if err != nil {
		return sourceProvenance{}, err
	}
	return sourceProvenance{ResolvedRef: "git:" + format + ":" + resolved, TreeDigest: "git:" + format + ":" + tree}, nil
}

func (s *Service) materializeOCI(ctx context.Context, root string, input ImportInput) (sourceProvenance, error) {
	registry, repository, reference, err := parseOCI(input.SourceURI, input.SourceRef)
	if err != nil {
		return sourceProvenance{}, err
	}
	secret := ""
	if input.CredentialEnv != "" {
		secret = os.Getenv(input.CredentialEnv)
		if secret == "" {
			return sourceProvenance{}, fmt.Errorf("credential env %s is not set", input.CredentialEnv)
		}
	}
	manifest, contentType, err := s.ociRequest(ctx, registry, repository, "manifests/"+reference, "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json", secret)
	if err != nil {
		return sourceProvenance{}, err
	}
	var index struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if strings.Contains(contentType, "index") || bytes.Contains(manifest, []byte(`"manifests"`)) {
		if err := json.Unmarshal(manifest, &index); err != nil || len(index.Manifests) == 0 {
			return sourceProvenance{}, fmt.Errorf("invalid OCI index")
		}
		manifest, _, err = s.ociRequest(ctx, registry, repository, "manifests/"+index.Manifests[0].Digest, "application/vnd.oci.image.manifest.v1+json", secret)
		if err != nil {
			return sourceProvenance{}, err
		}
	}
	var image struct {
		Layers []struct {
			Digest    string `json:"digest"`
			MediaType string `json:"mediaType"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(manifest, &image); err != nil || len(image.Layers) == 0 {
		return sourceProvenance{}, fmt.Errorf("invalid OCI image manifest")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return sourceProvenance{}, err
	}
	for _, layer := range image.Layers {
		blob, _, err := s.ociRequest(ctx, registry, repository, "blobs/"+layer.Digest, "application/octet-stream", secret)
		if err != nil {
			return sourceProvenance{}, err
		}
		if err := extractLayer(root, blob, strings.Contains(layer.MediaType, "gzip") || strings.HasSuffix(layer.MediaType, "+gzip")); err != nil {
			return sourceProvenance{}, err
		}
	}
	digest := sha256.Sum256(manifest)
	return sourceProvenance{ResolvedRef: "sha256:" + hex.EncodeToString(digest[:]), TreeDigest: "sha256:" + hex.EncodeToString(digest[:])}, nil
}

func gitMetadata(ctx context.Context, root string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read git provenance: %w", err)
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", fmt.Errorf("empty git provenance")
	}
	return value, nil
}

func (s *Service) ociRequest(ctx context.Context, registry, repository, path, accept, secret string) ([]byte, string, error) {
	url := "https://" + registry + "/v2/" + repository + "/" + path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Accept", accept)
	if secret != "" {
		request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(secret)))
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized && strings.HasPrefix(response.Header.Get("WWW-Authenticate"), "Bearer ") {
		token, err := s.ociToken(ctx, response.Header.Get("WWW-Authenticate"), secret)
		if err != nil {
			return nil, "", err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response.Body.Close()
		response, err = s.httpClient.Do(request)
		if err != nil {
			return nil, "", err
		}
		defer response.Body.Close()
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("OCI registry returned %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSkillBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxSkillBytes {
		return nil, "", fmt.Errorf("OCI response exceeds skill size limit")
	}
	return body, response.Header.Get("Content-Type"), nil
}

func (s *Service) ociToken(ctx context.Context, challenge, secret string) (string, error) {
	params := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(challenge, "Bearer "), ",") {
		pieces := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pieces) == 2 {
			params[pieces[0]] = strings.Trim(pieces[1], `"`)
		}
	}
	realm := params["realm"]
	if realm == "" || !strings.HasPrefix(realm, "https://") {
		return "", fmt.Errorf("invalid OCI bearer realm")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, realm, nil)
	if err != nil {
		return "", err
	}
	query := request.URL.Query()
	for _, key := range []string{"service", "scope"} {
		if params[key] != "" {
			query.Set(key, params[key])
		}
	}
	request.URL.RawQuery = query.Encode()
	if secret != "" {
		request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(secret)))
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body) != nil {
		return "", fmt.Errorf("OCI token request failed")
	}
	if body.Token != "" {
		return body.Token, nil
	}
	if body.AccessToken != "" {
		return body.AccessToken, nil
	}
	return "", fmt.Errorf("OCI token missing")
}

func parseOCI(uri, ref string) (string, string, string, error) {
	value := strings.TrimPrefix(uri, "oci://")
	if value == uri || strings.Contains(value, "@") || !strings.Contains(value, "/") {
		return "", "", "", fmt.Errorf("OCI source must be oci://registry/repository[:tag]")
	}
	registry, remainder, _ := strings.Cut(value, "/")
	if registry == "" || remainder == "" {
		return "", "", "", fmt.Errorf("invalid OCI source")
	}
	if ref == "" {
		if index := strings.LastIndex(remainder, ":"); index > strings.LastIndex(remainder, "/") {
			ref, remainder = remainder[index+1:], remainder[:index]
		} else {
			ref = "latest"
		}
	}
	return registry, remainder, ref, nil
}

func discoverReadmeRepositories(root, currentSource string) ([]string, CatalogueStats, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, CatalogueStats{}, err
	}
	for _, entry := range entries {
		if !strings.EqualFold(entry.Name(), "readme.md") || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, CatalogueStats{}, err
		}
		if !info.Mode().IsRegular() || info.Size() > maxFileBytes {
			return nil, CatalogueStats{}, fmt.Errorf("README.md is not a safe catalogue file")
		}
		body, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, CatalogueStats{}, err
		}
		repositories, stats := linkedRepositories(string(body), currentSource)
		return repositories, stats, nil
	}
	return nil, CatalogueStats{}, nil
}

func linkedRepositories(readme, currentSource string) ([]string, CatalogueStats) {
	current, _ := canonicalGitHubRepository(currentSource)
	seen := map[string]bool{}
	repositories := []string{}
	stats := CatalogueStats{}
	for _, raw := range githubRepositoryURL.FindAllString(readme, -1) {
		repository, ok := canonicalGitHubRepository(raw)
		if !ok || repository == current || seen[repository] {
			continue
		}
		seen[repository] = true
		stats.RepositoriesDiscovered++
		if len(repositories) >= maxCatalogueRepos {
			stats.RepositoriesSkipped++
			continue
		}
		repositories = append(repositories, repository)
	}
	return repositories, stats
}

func canonicalGitHubRepository(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimRight(raw, ".,;:"))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") || parsed.User != nil || parsed.Port() != "" {
		return "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return "", false
	}
	if len(parts) > 2 && parts[2] != "tree" && parts[2] != "blob" {
		return "", false
	}
	owner, repository := parts[0], strings.TrimSuffix(parts[1], ".git")
	if !validGitHubSegment(owner) || !validGitHubSegment(repository) {
		return "", false
	}
	return "https://github.com/" + owner + "/" + repository + ".git", true
}

func validGitHubSegment(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func discoverSkills(root string) ([]discoveredSkill, error) {
	candidates := []discoveredSkill{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() || !strings.EqualFold(info.Name(), "skill.md") {
			return nil
		}
		if len(candidates) >= maxSkillsPerImport {
			return fmt.Errorf("skill source exceeds %d discovered skills", maxSkillsPerImport)
		}
		bundleRoot := filepath.Dir(path)
		relative, err := filepath.Rel(root, bundleRoot)
		if err != nil || strings.HasPrefix(relative, "..") {
			return fmt.Errorf("unsafe skill path")
		}
		candidates = append(candidates, discoveredSkill{Root: bundleRoot, Path: filepath.ToSlash(relative)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
	return candidates, nil
}

func inspect(root string) (domain.SkillPackage, map[string]any, error) {
	pkg, findings, _, err := inspectCandidate(discoveredSkill{Root: root, Path: "."}, ImportInput{Mode: "native"})
	return pkg, findings, err
}

func inspectCandidate(candidate discoveredSkill, input ImportInput) (domain.SkillPackage, map[string]any, string, error) {
	root := candidate.Root
	files := map[string][]byte{}
	total := int64(0)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
			return fmt.Errorf("skill contains executable or non-regular file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(relative, "..") {
			return fmt.Errorf("unsafe skill path")
		}
		if len(files) >= maxSkillFiles {
			return fmt.Errorf("skill exceeds file limit")
		}
		if info.Size() > maxFileBytes {
			return fmt.Errorf("skill file exceeds size limit")
		}
		total += info.Size()
		if total > maxSkillBytes {
			return fmt.Errorf("skill exceeds size limit")
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = contents
		return nil
	})
	if err != nil {
		return domain.SkillPackage{}, nil, "", err
	}
	skillPath, skillBody := matchingFile(files, "skill.md")
	if skillPath == "" || len(skillBody) == 0 {
		return domain.SkillPackage{}, nil, "", fmt.Errorf("skill requires SKILL.md")
	}
	manifestPath, manifestBody := matchingFile(files, "skill.json")
	mode := input.Mode
	if mode == "auto" {
		if manifestPath != "" {
			mode = "native"
		} else {
			mode = "adapted"
		}
	}
	var manifest struct {
		ID          string           `json:"id"`
		Version     string           `json:"version"`
		Name        string           `json:"name"`
		Description string           `json:"description"`
		Tools       []map[string]any `json:"tools"`
	}
	metadata := map[string]any{}
	if mode == "native" {
		if manifestPath == "" {
			return domain.SkillPackage{}, nil, "", fmt.Errorf("native skill requires skill.json")
		}
		if err := json.Unmarshal(manifestBody, &manifest); err != nil {
			return domain.SkillPackage{}, nil, "", fmt.Errorf("invalid skill.json: %w", err)
		}
		if manifest.ID == "" || manifest.Version == "" || manifest.Name == "" {
			return domain.SkillPackage{}, nil, "", fmt.Errorf("skill.json requires id, version, name")
		}
		for _, tool := range manifest.Tools {
			if tool["id"] == nil || tool["kind"] == nil {
				return domain.SkillPackage{}, nil, "", fmt.Errorf("skill tool requires id and kind")
			}
		}
		if err := json.Unmarshal(manifestBody, &metadata); err != nil {
			return domain.SkillPackage{}, nil, "", err
		}
	} else {
		frontmatter := markdownFrontmatter(skillBody)
		manifest.ID = "adapted-" + skillSlug(input.SourceURI+"-"+candidate.Path)
		manifest.Version = frontmatter["version"]
		if manifest.Version == "" {
			manifest.Version = input.SourceRef
		}
		if manifest.Version == "" {
			manifest.Version = "unversioned"
		}
		manifest.Name = frontmatter["name"]
		if manifest.Name == "" {
			manifest.Name = strings.ReplaceAll(filepath.Base(candidate.Path), "-", " ")
			if candidate.Path == "." {
				manifest.Name = skillSlug(input.SourceURI)
			}
		}
		manifest.Description = frontmatter["description"]
		if manifest.Description == "" {
			manifest.Description = "Adapted external skill instructions."
		}
		metadata = map[string]any{"frontmatter": frontmatter, "tools": []any{}, "capabilities": []any{}}
	}
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		_, _ = hash.Write([]byte(key))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(files[key])
		_, _ = hash.Write([]byte{0})
	}
	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	metadata["mode"] = mode
	metadata["source_path"] = candidate.Path
	metadata["skill_file"] = skillPath
	findings := map[string]any{"status": "pass", "file_count": len(files), "bytes": total, "digest": digest, "validator": "skill-v2", "mode": mode, "path": candidate.Path, "name": manifest.Name, "id": manifest.ID, "version": manifest.Version}
	return domain.SkillPackage{Digest: digest, ID: manifest.ID, Version: manifest.Version, Name: manifest.Name, Description: manifest.Description, Manifest: metadata}, findings, mode, nil
}

func matchingFile(files map[string][]byte, target string) (string, []byte) {
	for path, body := range files {
		if strings.EqualFold(path, target) {
			return path, body
		}
	}
	return "", nil
}

func markdownFrontmatter(body []byte) map[string]string {
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return map[string]string{}
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return map[string]string{}
	}
	values := map[string]string{}
	for _, line := range strings.Split(text[4:4+end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) != "" {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	return values
}

func skillSlug(value string) string {
	var builder strings.Builder
	previousDash := false
	for _, runeValue := range strings.ToLower(value) {
		if runeValue >= 'a' && runeValue <= 'z' || runeValue >= '0' && runeValue <= '9' {
			builder.WriteRune(runeValue)
			previousDash = false
		} else if !previousDash {
			builder.WriteByte('-')
			previousDash = true
		}
	}
	value = strings.Trim(builder.String(), "-")
	if value == "" {
		return "external-skill"
	}
	return value[:min(len(value), 96)]
}

func copyBundle(source, destination string) error {
	if _, err := os.Stat(destination); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return err
	}
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o640)
	})
}

func extractLayer(root string, blob []byte, compressed bool) error {
	reader := io.Reader(bytes.NewReader(blob))
	if compressed {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader = gz
	}
	archive := tar.NewReader(io.LimitReader(reader, maxSkillBytes+1))
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > maxFileBytes {
			return fmt.Errorf("OCI skill layer contains unsafe entry")
		}
		path := filepath.Clean(header.Name)
		if path == "." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
			return fmt.Errorf("OCI skill layer path is unsafe")
		}
		target := filepath.Join(root, path)
		if !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("OCI skill layer path escapes root")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, io.LimitReader(archive, maxFileBytes+1))
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}

func validEnvName(value string) bool {
	for index, r := range value {
		if !(r == '_' || r >= 'A' && r <= 'Z' || index > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return value != ""
}
