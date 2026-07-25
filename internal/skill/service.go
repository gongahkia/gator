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
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/store"
)

const (
	maxSkillFiles = 128
	maxSkillBytes = 10 << 20
	maxFileBytes  = 1 << 20
)

type ImportInput struct {
	SourceType    string `json:"source_type"`
	SourceURI     string `json:"source_uri"`
	SourceRef     string `json:"source_ref"`
	CredentialEnv string `json:"credential_env"`
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
	input.SourceType, input.SourceURI, input.SourceRef, input.CredentialEnv = strings.TrimSpace(input.SourceType), strings.TrimSpace(input.SourceURI), strings.TrimSpace(input.SourceRef), strings.TrimSpace(input.CredentialEnv)
	if input.SourceType != "git" && input.SourceType != "oci" {
		return domain.SkillImport{}, domain.SkillPackage{}, fmt.Errorf("source_type must be git or oci")
	}
	if input.SourceURI == "" {
		return domain.SkillImport{}, domain.SkillPackage{}, fmt.Errorf("source_uri is required")
	}
	if input.CredentialEnv != "" && !validEnvName(input.CredentialEnv) {
		return domain.SkillImport{}, domain.SkillPackage{}, fmt.Errorf("credential_env is invalid")
	}
	temporary, err := os.MkdirTemp("", "norbot-skill-")
	if err != nil {
		return domain.SkillImport{}, domain.SkillPackage{}, err
	}
	defer os.RemoveAll(temporary)
	root := filepath.Join(temporary, "bundle")
	switch input.SourceType {
	case "git":
		err = materializeGit(ctx, root, input)
	case "oci":
		err = s.materializeOCI(ctx, root, input)
	}
	if err != nil {
		return domain.SkillImport{}, domain.SkillPackage{}, err
	}
	pkg, findings, err := inspect(root)
	if err != nil {
		return domain.SkillImport{}, domain.SkillPackage{}, err
	}
	destination := filepath.Join(s.artifactsDir, strings.TrimPrefix(pkg.Digest, "sha256:"))
	if err := copyBundle(root, destination); err != nil {
		return domain.SkillImport{}, domain.SkillPackage{}, err
	}
	pkg.Path = destination
	pkg, err = s.store.UpsertSkillPackage(ctx, pkg)
	if err != nil {
		return domain.SkillImport{}, domain.SkillPackage{}, err
	}
	imported, err := s.store.CreateSkillImport(ctx, domain.SkillImport{SourceType: input.SourceType, SourceURI: input.SourceURI, SourceRef: input.SourceRef, CredentialEnv: input.CredentialEnv, Digest: pkg.Digest, State: "scanned", Findings: findings})
	if err != nil {
		return domain.SkillImport{}, domain.SkillPackage{}, err
	}
	return imported, pkg, nil
}

func (s *Service) Activate(ctx context.Context, id int64) (domain.SkillImport, error) {
	return s.store.ActivateSkillImport(ctx, id)
}

func (s *Service) Imports(ctx context.Context) ([]domain.SkillImport, error) {
	return s.store.SkillImports(ctx)
}

func materializeGit(ctx context.Context, root string, input ImportInput) error {
	if !strings.HasPrefix(input.SourceURI, "https://") {
		return fmt.Errorf("git skill source must use https")
	}
	args := []string{"clone", "--depth", "1"}
	if input.SourceRef != "" {
		args = append(args, "--branch", input.SourceRef)
	}
	args = append(args, input.SourceURI, root)
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if input.CredentialEnv != "" {
		secret := os.Getenv(input.CredentialEnv)
		if secret == "" {
			return fmt.Errorf("credential env %s is not set", input.CredentialEnv)
		}
		command.Env = append(command.Env, "GIT_HTTP_EXTRAHEADER=Authorization: Basic "+base64.StdEncoding.EncodeToString([]byte(secret)))
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("clone skill source: %s", strings.TrimSpace(string(output)))
	}
	return os.RemoveAll(filepath.Join(root, ".git"))
}

func (s *Service) materializeOCI(ctx context.Context, root string, input ImportInput) error {
	registry, repository, reference, err := parseOCI(input.SourceURI, input.SourceRef)
	if err != nil {
		return err
	}
	secret := ""
	if input.CredentialEnv != "" {
		secret = os.Getenv(input.CredentialEnv)
		if secret == "" {
			return fmt.Errorf("credential env %s is not set", input.CredentialEnv)
		}
	}
	manifest, contentType, err := s.ociRequest(ctx, registry, repository, "manifests/"+reference, "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json", secret)
	if err != nil {
		return err
	}
	var index struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if strings.Contains(contentType, "index") || bytes.Contains(manifest, []byte(`"manifests"`)) {
		if err := json.Unmarshal(manifest, &index); err != nil || len(index.Manifests) == 0 {
			return fmt.Errorf("invalid OCI index")
		}
		manifest, _, err = s.ociRequest(ctx, registry, repository, "manifests/"+index.Manifests[0].Digest, "application/vnd.oci.image.manifest.v1+json", secret)
		if err != nil {
			return err
		}
	}
	var image struct {
		Layers []struct {
			Digest    string `json:"digest"`
			MediaType string `json:"mediaType"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(manifest, &image); err != nil || len(image.Layers) == 0 {
		return fmt.Errorf("invalid OCI image manifest")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	for _, layer := range image.Layers {
		blob, _, err := s.ociRequest(ctx, registry, repository, "blobs/"+layer.Digest, "application/octet-stream", secret)
		if err != nil {
			return err
		}
		if err := extractLayer(root, blob, strings.Contains(layer.MediaType, "gzip") || strings.HasSuffix(layer.MediaType, "+gzip")); err != nil {
			return err
		}
	}
	return nil
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

func inspect(root string) (domain.SkillPackage, map[string]any, error) {
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
		return domain.SkillPackage{}, nil, err
	}
	if len(files["SKILL.md"]) == 0 || len(files["skill.json"]) == 0 {
		return domain.SkillPackage{}, nil, fmt.Errorf("skill requires SKILL.md and skill.json")
	}
	var manifest struct {
		ID          string           `json:"id"`
		Version     string           `json:"version"`
		Name        string           `json:"name"`
		Description string           `json:"description"`
		Tools       []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(files["skill.json"], &manifest); err != nil {
		return domain.SkillPackage{}, nil, fmt.Errorf("invalid skill.json: %w", err)
	}
	if manifest.ID == "" || manifest.Version == "" || manifest.Name == "" {
		return domain.SkillPackage{}, nil, fmt.Errorf("skill.json requires id, version, name")
	}
	for _, tool := range manifest.Tools {
		if tool["id"] == nil || tool["kind"] == nil {
			return domain.SkillPackage{}, nil, fmt.Errorf("skill tool requires id and kind")
		}
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
	metadata := map[string]any{}
	if err := json.Unmarshal(files["skill.json"], &metadata); err != nil {
		return domain.SkillPackage{}, nil, err
	}
	return domain.SkillPackage{Digest: digest, ID: manifest.ID, Version: manifest.Version, Name: manifest.Name, Description: manifest.Description, Manifest: metadata}, map[string]any{"status": "pass", "file_count": len(files), "bytes": total, "digest": digest, "validator": "skill-v1"}, nil
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
