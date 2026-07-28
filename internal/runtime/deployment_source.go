package runtime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gongahkia/norbot/internal/domain"
)

const deploymentDirectory = ".norbot/deployment"

type ArchivedDeploymentFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type DeploymentSourceReport struct {
	Archived  []ArchivedDeploymentFile `json:"archived,omitempty"`
	Protected []string                 `json:"protected"`
}

func PrepareDeploymentSource(runRoot string, run domain.Run) (DeploymentSourceReport, error) {
	appRoot := filepath.Join(runRoot, "generated-app")
	if err := validateApplicationSource(appRoot, run); err != nil {
		return DeploymentSourceReport{}, err
	}
	serverFiles := serverOwnedFiles(run)
	expected := make(map[string]string, len(serverFiles))
	for relative, content := range serverFiles {
		expected[filepath.Join(appRoot, relative)] = content
	}
	paths, err := reservedDeploymentPaths(appRoot, expected)
	if err != nil {
		return DeploymentSourceReport{}, err
	}
	report := DeploymentSourceReport{Protected: make([]string, 0, len(expected))}
	for path := range expected {
		rel, _ := filepath.Rel(appRoot, path)
		report.Protected = append(report.Protected, filepath.ToSlash(rel))
	}
	sort.Strings(report.Protected)
	for _, path := range paths {
		if err := archiveDeploymentFile(runRoot, path, &report); err != nil {
			return DeploymentSourceReport{}, err
		}
	}
	for path, content := range expected {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return DeploymentSourceReport{}, err
		}
		if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
			return DeploymentSourceReport{}, err
		}
	}
	return report, nil
}

func validateApplicationSource(appRoot string, run domain.Run) error {
	required := []string{"frontend/package.json", "frontend/package-lock.json", "frontend/src/main.jsx"}
	if run.Profile != domain.ProfileFrontend {
		required = append(required, "backend/main.go", "backend/go.mod", "backend/go.sum")
	}
	if run.Profile == domain.ProfileAgentic {
		required = append(required, "backend/AGENT_RUNTIME.md")
	}
	for _, relative := range required {
		info, err := os.Stat(filepath.Join(appRoot, relative))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("generated app missing %s", relative)
		}
	}
	return nil
}

func serverOwnedFiles(run domain.Run) map[string]string {
	root := filepath.FromSlash(deploymentDirectory)
	files := map[string]string{
		filepath.Join(root, "frontend.Dockerfile"): `FROM node:22-alpine@sha256:16e22a550f3863206a3f701448c45f7912c6896a62de43add43bb9c86130c3e2 AS build
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --ignore-scripts
COPY frontend/ ./
RUN npm run build

FROM caddy:2.10-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d AS caddy
RUN setcap -r /usr/bin/caddy

FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d
COPY --from=caddy /usr/bin/caddy /usr/bin/caddy
WORKDIR /srv
COPY --from=build --chown=10001:10001 /app/dist /srv
ENV XDG_CONFIG_HOME=/tmp XDG_DATA_HOME=/tmp
USER 10001:10001
EXPOSE 8080
CMD ["caddy","file-server","--root","/srv","--listen",":8080"]
`,
	}
	if run.Profile != domain.ProfileFrontend {
		files[filepath.Join(root, "backend.Dockerfile")] = `FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/server .

FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d
COPY --from=build /out/server /server
USER 65532:65532
EXPOSE 8000
ENTRYPOINT ["/server"]
`
	}
	acceptance, _ := json.MarshalIndent(run.Architecture.Acceptance, "", "  ")
	files[filepath.Join(".norbot", "acceptance", "contract.json")] = string(acceptance)
	return files
}

func reservedDeploymentPaths(appRoot string, expected map[string]string) ([]string, error) {
	paths := []string{}
	err := filepath.Walk(appRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular generated file %s", path)
		}
		rel, err := filepath.Rel(appRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		fullExpected := filepath.Join(appRoot, filepath.FromSlash(rel))
		if wanted, ok := expected[fullExpected]; ok {
			actual, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if string(actual) != wanted {
				paths = append(paths, path)
			}
			return nil
		}
		if strings.HasPrefix(rel, ".norbot/") || isModelDeploymentDescriptor(rel) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func isModelDeploymentDescriptor(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	if base == "Dockerfile" || base == ".dockerignore" {
		return true
	}
	return strings.HasPrefix(lower, "docker-compose") && (strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml"))
}

func archiveDeploymentFile(runRoot, path string, report *DeploymentSourceReport) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(runRoot, path)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(filepath.ToSlash(rel), "generated-app/") {
		return fmt.Errorf("unsafe deployment descriptor path")
	}
	archiveRoot, err := os.MkdirTemp(runRoot, "deployment-archive-")
	if err != nil {
		return err
	}
	target := filepath.Join(archiveRoot, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	if err := os.Rename(path, target); err != nil {
		return err
	}
	report.Archived = append(report.Archived, ArchivedDeploymentFile{Path: filepath.ToSlash(rel), Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(content))})
	return nil
}
