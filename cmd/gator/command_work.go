package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/workrun"
)

const maxStdinObjectiveBytes = 64 * 1024

type workModelFactory func(provider, modelName, baseURL string) (agent.Model, error)

func workTask(arguments []string, out io.Writer) error {
	return runWorkTask(arguments, os.Stdin, out, nativeWorkModel)
}

func nativeWorkModel(provider, modelName, baseURL string) (agent.Model, error) {
	executor, err := newExecutor(provider, modelName, baseURL)
	if err != nil {
		return nil, err
	}
	return executor.Model, nil
}

func runWorkTask(arguments []string, in io.Reader, out io.Writer, modelFactory workModelFactory) error {
	flags := flag.NewFlagSet("work", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	defaults, err := configuredDefaults()
	if err != nil {
		return err
	}
	defaultProvider := valueOrDefault(os.Getenv("GATOR_PROVIDER"), defaults.Provider)
	defaultModel := valueOrDefault(os.Getenv("GATOR_MODEL"), defaults.Model)
	providerName := flags.String("provider", defaultProvider, "model provider")
	modelName := flags.String("model", defaultModel, "model name")
	baseURL := flags.String("base-url", os.Getenv("GATOR_BASE_URL"), "provider API base URL override")
	sourcePath := flags.String("source", ".", "read-only source directory")
	modeName := flags.String("mode", string(action.Draft), "work mode: inspect, draft, or act")
	maxSteps := flags.Int("max-steps", 24, "maximum model turns")
	runID := flags.String("run-id", "", "stable run identifier")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable result")
	var artifacts artifactFlags
	flags.Var(&artifacts, "artifact", "required output-relative artifact path (repeatable; default report.md)")
	var contains containsFlags
	flags.Var(&contains, "require-contains", "required literal as ARTIFACT=TEXT (repeatable)")
	var selectedConnectors connectorFlags
	flags.Var(&selectedConnectors, "connector", "configured connected source ID (repeatable)")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	objective, err := workObjective(flags.Args(), in)
	if err != nil {
		return err
	}
	mode := action.Mode(strings.ToLower(strings.TrimSpace(*modeName)))
	if err := mode.Validate(); err != nil {
		return err
	}
	contract, err := workContract(mode, artifacts, contains)
	if err != nil {
		return err
	}
	resolvedProvider, resolvedModel, err := resolveConfiguredProvider(*providerName, *modelName)
	if err != nil {
		return err
	}
	backend, err := modelFactory(resolvedProvider, resolvedModel, *baseURL)
	if err != nil {
		return err
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	if !*jsonOutput {
		if _, err := fmt.Fprintf(out, "Gator Work\n  provider: %s\n  model: %s\n  mode: %s\n  source: %s\n  connectors: %s\n  objective: %s\n", resolvedProvider, displayModel(resolvedModel), mode, *sourcePath, valueOrDash(strings.Join(selectedConnectors, ", ")), objective); err != nil {
			return err
		}
	}
	var sink agent.EventSink
	if !*jsonOutput {
		printer := &eventPrinter{out: out}
		sink = printer.Print
	}
	executor := workrun.Executor{Model: backend, StateDir: stateDir, Connectors: connector.Runtime{Registry: registry, Credentials: credentials}}
	outcome, runErr := executor.Execute(context.Background(), workrun.Request{
		SourcePath:   *sourcePath,
		Objective:    objective,
		RunID:        *runID,
		MaxSteps:     *maxSteps,
		Mode:         mode,
		Contract:     contract,
		OnEvent:      sink,
		ConnectorIDs: selectedConnectors,
	})
	if *jsonOutput {
		if err := writeWorkJSON(out, outcome, runErr); err != nil {
			return err
		}
	} else if outcome.Work.Path != "" {
		if err := writeWorkSummary(out, outcome); err != nil && runErr == nil {
			runErr = err
		}
	}
	return runErr
}

func workObjective(arguments []string, in io.Reader) (string, error) {
	objective := strings.TrimSpace(strings.Join(arguments, " "))
	if objective != "" {
		return objective, nil
	}
	contents, err := io.ReadAll(io.LimitReader(in, maxStdinObjectiveBytes+1))
	if err != nil {
		return "", fmt.Errorf("read work objective from stdin: %w", err)
	}
	if len(contents) > maxStdinObjectiveBytes {
		return "", errors.New("work objective from stdin exceeds 64 KiB")
	}
	objective = strings.TrimSpace(string(contents))
	if objective == "" {
		return "", errors.New("work objective is required as arguments or stdin")
	}
	return objective, nil
}

func workContract(mode action.Mode, paths artifactFlags, contains containsFlags) (artifact.Contract, error) {
	if mode == action.Inspect {
		if len(paths) > 0 || len(contains) > 0 {
			return artifact.Contract{}, errors.New("inspect mode does not accept artifact requirements")
		}
		return artifact.InspectionContract(), nil
	}
	if len(paths) == 0 {
		paths = artifactFlags{"report.md"}
	}
	seen := make(map[string]struct{}, len(paths))
	requirements := make([]artifact.Requirement, 0, len(paths))
	for _, path := range paths {
		if err := artifact.ValidateOutputPath(path); err != nil {
			return artifact.Contract{}, err
		}
		if _, duplicate := seen[path]; duplicate {
			return artifact.Contract{}, fmt.Errorf("artifact path %q is repeated", path)
		}
		seen[path] = struct{}{}
		requirement := artifact.Requirement{
			Path:        path,
			Validations: []artifact.Validation{{Kind: artifact.ArtifactExists}, {Kind: artifact.NonEmpty}},
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".markdown":
			requirement.MediaTypes = []string{"text/markdown"}
			requirement.Validations = append(requirement.Validations, artifact.Validation{Kind: artifact.UTF8})
		case ".txt":
			requirement.MediaTypes = []string{"text/plain"}
			requirement.Validations = append(requirement.Validations, artifact.Validation{Kind: artifact.UTF8})
		case ".json":
			requirement.MediaTypes = []string{"application/json"}
			requirement.Validations = append(requirement.Validations, artifact.Validation{Kind: artifact.UTF8}, artifact.Validation{Kind: artifact.JSON})
		case ".csv":
			requirement.MediaTypes = []string{"text/csv"}
			requirement.Validations = append(requirement.Validations, artifact.Validation{Kind: artifact.UTF8}, artifact.Validation{Kind: artifact.CSV})
		case ".yaml", ".yml":
			requirement.MediaTypes = []string{"application/yaml"}
			requirement.Validations = append(requirement.Validations, artifact.Validation{Kind: artifact.UTF8})
		}
		requirements = append(requirements, requirement)
	}
	for _, condition := range contains {
		path, literal, found := strings.Cut(condition, "=")
		path = strings.TrimSpace(path)
		if !found || path == "" || literal == "" {
			return artifact.Contract{}, fmt.Errorf("invalid --require-contains %q; expected ARTIFACT=TEXT", condition)
		}
		if _, exists := seen[path]; !exists {
			return artifact.Contract{}, fmt.Errorf("--require-contains references undeclared artifact %q", path)
		}
		for index := range requirements {
			if requirements[index].Path == path {
				requirements[index].Validations = append(requirements[index].Validations, artifact.Validation{Kind: artifact.Contains, Value: literal})
			}
		}
	}
	contract := artifact.Contract{
		Version:          artifact.ContractVersion,
		Artifacts:        requirements,
		MaxArtifactBytes: artifact.DefaultMaxArtifactBytes,
		MaxTotalBytes:    artifact.DefaultMaxTotalBytes,
		ExternalActions:  action.Forbid,
	}
	if err := contract.Validate(); err != nil {
		return artifact.Contract{}, err
	}
	return contract, nil
}

func writeWorkSummary(out io.Writer, outcome workrun.Outcome) error {
	if _, err := fmt.Fprintf(out, "\nWork: %s\nStaged output: %s\nManifest: %s\nStatus: %s\n", outcome.Work.ID, outcome.Work.Output.Path(), outcome.Work.ManifestPath, outcome.Manifest.Status); err != nil {
		return err
	}
	for _, file := range outcome.Manifest.Artifacts {
		if _, err := fmt.Fprintf(out, "  ✓ %s (%s, %d bytes, sha256:%s)\n", file.Path, file.MediaType, file.Bytes, file.SHA256[:12]); err != nil {
			return err
		}
	}
	if outcome.Result.FinalText != "" {
		_, err := fmt.Fprintf(out, "\n%s\n", outcome.Result.FinalText)
		return err
	}
	return nil
}

func writeWorkJSON(out io.Writer, outcome workrun.Outcome, runErr error) error {
	type response struct {
		RunID        string                      `json:"run_id,omitempty"`
		Status       artifact.Status             `json:"status,omitempty"`
		OutputPath   string                      `json:"output_path,omitempty"`
		ManifestPath string                      `json:"manifest_path,omitempty"`
		Artifacts    []artifact.File             `json:"artifacts,omitempty"`
		Validations  []artifact.ValidationResult `json:"validations,omitempty"`
		FinalText    string                      `json:"final_text,omitempty"`
		Error        string                      `json:"error,omitempty"`
	}
	result := response{
		RunID: outcome.Work.ID, Status: outcome.Manifest.Status,
		ManifestPath: outcome.Work.ManifestPath, Artifacts: outcome.Manifest.Artifacts,
		Validations: outcome.Manifest.Validations, FinalText: outcome.Result.FinalText,
	}
	if outcome.Work.Output.Path() != "" {
		result.OutputPath = outcome.Work.Output.Path()
	}
	if runErr != nil {
		result.Error = runErr.Error()
	}
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}

type artifactFlags []string

func (f *artifactFlags) String() string { return strings.Join(*f, ", ") }

func (f *artifactFlags) Set(value string) error {
	if err := artifact.ValidateOutputPath(value); err != nil {
		return err
	}
	*f = append(*f, value)
	return nil
}

type containsFlags []string

func (f *containsFlags) String() string { return strings.Join(*f, ", ") }

func (f *containsFlags) Set(value string) error {
	if len(value) > 8*1024 || strings.ContainsRune(value, 0) {
		return errors.New("artifact literal requirement is invalid")
	}
	*f = append(*f, value)
	return nil
}

type connectorFlags []string

func (f *connectorFlags) String() string { return strings.Join(*f, ", ") }

func (f *connectorFlags) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("connector ID is invalid")
	}
	*f = append(*f, value)
	return nil
}

func valueOrDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
