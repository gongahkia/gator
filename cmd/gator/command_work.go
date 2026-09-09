package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	gatorbrowser "github.com/gongahkia/gator/internal/browser"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

const maxStdinObjectiveBytes = 64 * 1024

type workModelFactory func(provider, modelName, baseURL string) (agent.Model, error)

func workTask(arguments []string, out io.Writer) error {
	if len(arguments) > 0 && isWorkSessionCommand(arguments[0]) {
		return workSessionCommand(arguments, os.Stdin, out, nativeWorkModel)
	}
	return runWorkTask(arguments, os.Stdin, out, nativeWorkModel)
}

func inspectTask(arguments []string, out io.Writer) error {
	return runInspectTask(arguments, os.Stdin, out, nativeWorkModel)
}

func runInspectTask(arguments []string, in io.Reader, out io.Writer, modelFactory workModelFactory) error {
	forwarded := make([]string, 0, len(arguments)+2)
	forwarded = append(forwarded, "--mode", string(action.Inspect))
	forwarded = append(forwarded, arguments...)
	return runWorkTask(forwarded, in, out, modelFactory)
}

func nativeWorkModel(provider, modelName, baseURL string) (agent.Model, error) {
	executor, err := newExecutor(provider, modelName, baseURL)
	if err != nil {
		return nil, err
	}
	return &nativeWorkBackend{Model: executor.Model, code: executor, provider: provider, model: modelName, baseURL: baseURL}, nil
}

type nativeWorkBackend struct {
	agent.Model
	code     gatorrun.Executor
	provider string
	model    string
	baseURL  string
}

func (b *nativeWorkBackend) CompleteStream(ctx context.Context, request agent.TurnRequest, delta func(string)) (agent.Turn, error) {
	if streaming, ok := b.Model.(agent.StreamingModel); ok {
		return streaming.CompleteStream(ctx, request, delta)
	}
	return b.Model.Complete(ctx, request)
}
func (b *nativeWorkBackend) SupportsVisualInput() bool {
	if visual, ok := b.Model.(agent.VisualInputModel); ok {
		return visual.SupportsVisualInput()
	}
	return true
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
	actionDisposition := flags.String("actions", string(action.Forbid), "external actions: forbid, draft, or approve")
	maxSteps := flags.Int("max-steps", 24, "maximum model turns")
	contractPath := flags.String("contract", "", "complete outcome contract JSON")
	requests := flags.Int("max-model-requests", 256, "aggregate manager and child requests")
	tokens := flags.Int64("max-tokens", 0, "reported token limit; zero disables")
	seconds := flags.Int("timeout-seconds", 1800, "aggregate wall time")
	codeMaxSteps := flags.Int("code-max-steps", 0, "maximum internal Code specialist turns")
	requireCode := flags.Bool("require-code", false, "require an internal Code specialist patch")
	runID := flags.String("run-id", "", "stable run identifier")
	conversationID := flags.String("conversation", "", "continue a retained Work conversation")
	parentRevisionID := flags.String("parent", "", "branch from a retained Work revision")
	refreshSource := flags.Bool("refresh-source", false, "capture a new immutable source snapshot")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable result")
	var artifacts artifactFlags
	flags.Var(&artifacts, "artifact", "required output-relative artifact path (repeatable; default report.md)")
	var contains containsFlags
	flags.Var(&contains, "require-contains", "required literal as ARTIFACT=TEXT (repeatable)")
	var webOrigins stringFlags
	flags.Var(&webOrigins, "web-origin", "explicitly permitted HTTPS research origin (repeatable)")
	var selectedConnectors connectorFlags
	flags.Var(&selectedConnectors, "connector", "configured connected source ID (repeatable)")
	var imagePaths attachmentFlags
	flags.Var(&imagePaths, "image", "source-relative PNG, JPEG, or WebP image to include in the initial prompt (repeatable)")
	var documentPaths attachmentFlags
	flags.Var(&documentPaths, "attach", "source-relative PDF or supported document/text file to include in the initial prompt (repeatable)")
	var codeVerification verificationFlags
	flags.Var(&codeVerification, "verify", "Code specialist verification argv (repeatable)")
	var codeScopes stringFlags
	flags.Var(&codeScopes, "scope", "Code specialist project-instruction scope (repeatable)")
	codeProfile := flags.String("profile", "", "Code specialist project profile")
	var codeSetup verificationFlags
	flags.Var(&codeSetup, "setup", "Code specialist worktree setup argv (repeatable)")
	var codeAllowed verificationFlags
	flags.Var(&codeAllowed, "allow-command", "pre-approve an exact Code specialist argv (repeatable)")
	var codePrefixes verificationFlags
	flags.Var(&codePrefixes, "allow-command-prefix", "pre-approve a literal Code specialist argv prefix (repeatable)")
	codeSandbox := flags.String("sandbox", string(sandbox.Strict), "Code specialist sandbox: strict or off")
	codeNetwork := flags.String("network", string(sandbox.DenyNetwork), "Code specialist network: deny or allow")
	var codeCapabilities stringFlags
	flags.Var(&codeCapabilities, "code-capability", "explicit Code grant: hooks, lsp, mcp, extension, http, browser, or terminal")
	codeBrowserSession := flags.String("browser-session", "", "explicit browser session granted to the Code specialist")
	legacyBase := flags.String("base", "", "removed Code worktree base override")
	legacyCopyIgnored := flags.Bool("copy-ignored", false, "removed Code ignored-file copy mode")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if strings.TrimSpace(*legacyBase) != "" || *legacyCopyIgnored {
		return errors.New("--base and --copy-ignored belonged to the standalone Code frontend; Gator now delegates against its immutable source snapshot")
	}
	objective, err := workObjective(flags.Args(), in)
	if err != nil {
		return err
	}
	mode := action.Mode(strings.ToLower(strings.TrimSpace(*modeName)))
	if err := mode.Validate(); err != nil {
		return err
	}
	disposition := action.Disposition(strings.ToLower(strings.TrimSpace(*actionDisposition)))
	contract, err := workContract(mode, disposition, artifacts, contains)
	if err != nil {
		return err
	}
	if *contractPath != "" {
		if err := readWorkJSON(*contractPath, &contract); err != nil {
			return err
		}
		disposition = contract.ExternalActions
	}
	if *jsonOutput && disposition == action.Approve {
		return errors.New("--json cannot request interactive action approval; use --actions draft or an interactive run")
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
	if strings.TrimSpace(*conversationID) != "" {
		sessions, err := worksession.Open(stateDir)
		if err != nil {
			return err
		}
		conversation, err := sessions.Load(strings.TrimSpace(*conversationID))
		if err != nil {
			return fmt.Errorf("load Work conversation: %w", err)
		}
		*sourcePath = conversation.SourcePath
	}
	images, attachments, err := loadPromptAttachments(*sourcePath, imagePaths, documentPaths)
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
		if _, err := fmt.Fprintf(out, "Gator Work\n  provider: %s\n  model: %s\n  mode: %s\n  external actions: %s\n  source: %s\n  connectors: %s\n  objective: %s\n", resolvedProvider, displayModel(resolvedModel), mode, disposition, *sourcePath, valueOrDash(strings.Join(selectedConnectors, ", ")), objective); err != nil {
			return err
		}
		if err := writePromptAttachmentSummary(out, images, attachments); err != nil {
			return err
		}
	}
	var sink agent.EventSink
	var snapshotSink func(snapshot.Manifest)
	if !*jsonOutput {
		printer := &eventPrinter{out: out}
		sink = printer.Print
		snapshotSink = func(manifest snapshot.Manifest) {
			_, _ = fmt.Fprintf(out, "  snapshot: %s (%d files, %d bytes, %d exclusions)\n", manifest.ID, manifest.Files, manifest.Bytes, len(manifest.Exclusions))
		}
	}
	executor := workrun.Executor{Model: backend, StateDir: stateDir, Connectors: connector.Runtime{Registry: registry, Credentials: credentials}}
	if native, ok := backend.(*nativeWorkBackend); ok {
		if strings.TrimSpace(*codeBrowserSession) != "" {
			if sandbox.Network(*codeNetwork) != sandbox.AllowNetwork {
				return errors.New("--browser-session requires --network allow for the internal Code specialist")
			}
			if !containsString(codeCapabilities, workrun.CodeCapabilityBrowser) {
				return errors.New("--browser-session requires --code-capability browser")
			}
			store, err := gatorbrowser.Open(stateDir)
			if err != nil {
				return err
			}
			client, err := gatorbrowser.NewClient(store, *codeBrowserSession)
			if err != nil {
				return err
			}
			session, err := client.Session(context.Background(), *codeBrowserSession)
			if err != nil {
				return err
			}
			if len(session.SelectedTabs) == 0 {
				return errors.New("--browser-session has no developer-selected tabs")
			}
			native.code.Browser = client
		}
		executor.Code = native.codeDelegate(stateDir)
	}
	if err := configureWorkRoles(&executor, settings, stateDir); err != nil {
		return err
	}
	if len(codeVerification) == 0 {
		codeVerification = parseSuggestedVerification(suggestedVerificationCommands(*sourcePath))
	}
	var approve action.Approver
	var approveRead func(context.Context, string, string, json.RawMessage) (bool, error)
	if !*jsonOutput {
		approveRead = workConnectorReadApprover(in, out)
	}
	if disposition == action.Approve {
		approve = workActionApprover(in, out)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	outcome, runErr := (workrun.Service{Executor: executor}).Execute(ctx, workrun.Request{
		Limits:               agent.Limits{ModelRequests: *requests, Tokens: *tokens, WallSeconds: *seconds},
		OTLPEndpoint:         os.Getenv("GATOR_OTLP_ENDPOINT"),
		WebOrigins:           webOrigins,
		SourcePath:           *sourcePath,
		Provider:             resolvedProvider + "/" + resolvedModel,
		Objective:            objective,
		RunID:                *runID,
		MaxSteps:             *maxSteps,
		Mode:                 mode,
		Contract:             contract,
		OnEvent:              sink,
		ConnectorIDs:         selectedConnectors,
		ApproveAction:        approve,
		ApproveConnectorRead: approveRead,
		ConversationID:       strings.TrimSpace(*conversationID), ParentRevisionID: strings.TrimSpace(*parentRevisionID), RefreshSource: *refreshSource,
		ConnectorPermissions: connector.PermissionSet(settings.ConnectorPermissions),
		SnapshotOptions:      snapshot.Options{Limits: snapshot.Limits{MaxFiles: settings.Snapshots.MaxFiles, MaxTotal: settings.Snapshots.MaxTotalBytes, MaxFileBytes: settings.Snapshots.MaxFileBytes}, Excludes: settings.Snapshots.Excludes},
		OnSnapshot:           snapshotSink,
		Images:               images,
		Attachments:          attachments,
		RequireCode:          *requireCode,
		Code: workrun.CodePolicy{
			MaxSteps: *codeMaxSteps, Verification: codeVerification, Scopes: codeScopes, Profile: strings.TrimSpace(*codeProfile),
			Setup: codeSetup, AllowedCommands: codeAllowed, AllowedCommandPrefixes: codePrefixes,
			Sandbox:      sandbox.Policy{Mode: sandbox.Mode(*codeSandbox), Network: sandbox.Network(*codeNetwork)},
			Capabilities: append([]string(nil), codeCapabilities...), BrowserSession: strings.TrimSpace(*codeBrowserSession),
		},
		ApproveCodeCommand: codeCommandApprover(*jsonOutput, in, out),
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

func workContract(mode action.Mode, disposition action.Disposition, paths artifactFlags, contains containsFlags) (artifact.Contract, error) {
	if err := disposition.Validate(); err != nil {
		return artifact.Contract{}, err
	}
	if mode == action.Inspect {
		if len(paths) > 0 || len(contains) > 0 || disposition != action.Forbid {
			return artifact.Contract{}, errors.New("inspect mode does not accept artifact requirements or external actions")
		}
		return artifact.InspectionContract(), nil
	}
	if mode == action.Draft && disposition == action.Approve {
		return artifact.Contract{}, errors.New("draft mode cannot approve external actions; use --actions draft or --mode act")
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
		case ".docx":
			requirement.MediaTypes = []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document"}
			requirement.Validations = append(requirement.Validations, artifact.Validation{Kind: artifact.DOCX})
		case ".xlsx":
			requirement.MediaTypes = []string{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}
			requirement.Validations = append(requirement.Validations, artifact.Validation{Kind: artifact.XLSX})
		case ".pdf":
			requirement.MediaTypes = []string{"application/pdf"}
			requirement.Validations = append(requirement.Validations, artifact.Validation{Kind: artifact.PDF})
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
		ExternalActions:  disposition,
	}
	if err := contract.Validate(); err != nil {
		return artifact.Contract{}, err
	}
	return contract, nil
}

func writeWorkSummary(out io.Writer, outcome workrun.Outcome) error {
	if _, err := fmt.Fprintf(out, "\nWork: %s\nConversation: %s\nRevision: %s\nSnapshot: %s\nStaged output: %s\nManifest: %s\nStatus: %s\n", outcome.Work.ID, outcome.ConversationID, outcome.RevisionID, outcome.SnapshotID, outcome.Work.Output.Path(), outcome.Work.ManifestPath, outcome.Manifest.Status); err != nil {
		return err
	}
	for _, file := range outcome.Manifest.Artifacts {
		if _, err := fmt.Fprintf(out, "  ✓ %s (%s, %d bytes, sha256:%s)\n", file.Path, file.MediaType, file.Bytes, file.SHA256[:12]); err != nil {
			return err
		}
	}
	for _, record := range outcome.Manifest.Actions {
		if _, err := fmt.Fprintf(out, "  → %s/%s: %s (%s, sha256:%s)\n", record.Proposal.ConnectorID, record.Proposal.Operation, record.Status, record.Proposal.Target, record.Proposal.PayloadSHA256[:12]); err != nil {
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
		RunID          string                      `json:"run_id,omitempty"`
		Status         artifact.Status             `json:"status,omitempty"`
		OutputPath     string                      `json:"output_path,omitempty"`
		ManifestPath   string                      `json:"manifest_path,omitempty"`
		Artifacts      []artifact.File             `json:"artifacts,omitempty"`
		Validations    []artifact.ValidationResult `json:"validations,omitempty"`
		Actions        []action.Record             `json:"actions,omitempty"`
		FinalText      string                      `json:"final_text,omitempty"`
		Error          string                      `json:"error,omitempty"`
		ConversationID string                      `json:"conversation_id,omitempty"`
		RevisionID     string                      `json:"revision_id,omitempty"`
		SnapshotID     string                      `json:"snapshot_id,omitempty"`
	}
	result := response{
		RunID: outcome.Work.ID, Status: outcome.Manifest.Status,
		ManifestPath: outcome.Work.ManifestPath, Artifacts: outcome.Manifest.Artifacts,
		Validations: outcome.Manifest.Validations, Actions: outcome.Manifest.Actions, FinalText: outcome.Result.FinalText,
		ConversationID: outcome.ConversationID, RevisionID: outcome.RevisionID, SnapshotID: outcome.SnapshotID,
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

func workActionApprover(in io.Reader, out io.Writer) action.Approver {
	return func(ctx context.Context, proposal action.Proposal) (action.Decision, error) {
		select {
		case <-ctx.Done():
			return action.Deny, ctx.Err()
		default:
		}
		if _, err := fmt.Fprintf(out, "\nExternal action approval\n  connector: %s\n  operation: %s (%s)\n  target: %s\n  payload sha256: %s\n  exact JSON (escaped): %s\nApprove this one action? [y/N] ", proposal.ConnectorID, proposal.Operation, proposal.Capability, proposal.Target, proposal.PayloadSHA256, strconv.QuoteToASCII(proposal.Preview)); err != nil {
			return action.Deny, err
		}
		line, err := readApprovalLine(in)
		if errors.Is(err, io.EOF) && line == "" {
			return action.Deny, nil
		}
		if err != nil && !(errors.Is(err, io.EOF) && line != "") {
			return action.Deny, err
		}
		if strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes") {
			return action.Allow, nil
		}
		return action.Deny, nil
	}
}

func workConnectorReadApprover(in io.Reader, out io.Writer) func(context.Context, string, string, json.RawMessage) (bool, error) {
	return func(ctx context.Context, connectorID, operation string, arguments json.RawMessage) (bool, error) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
		digest := sha256.Sum256(arguments)
		if _, err := fmt.Fprintf(out, "\nConnected read approval\n  connector: %s\n  operation: %s\n  request sha256: %x\n  exact JSON (escaped): %s\nAllow this one read? [y/N] ", connectorID, operation, digest, strconv.QuoteToASCII(string(arguments))); err != nil {
			return false, err
		}
		line, err := readApprovalLine(in)
		if errors.Is(err, io.EOF) && line == "" {
			return false, nil
		}
		if err != nil && !(errors.Is(err, io.EOF) && line != "") {
			return false, err
		}
		return strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes"), nil
	}
}

func readApprovalLine(in io.Reader) (string, error) {
	var line strings.Builder
	var one [1]byte
	for line.Len() < 4096 {
		count, err := in.Read(one[:])
		if count == 1 {
			if one[0] == '\n' {
				return line.String(), nil
			}
			line.WriteByte(one[0])
		}
		if err != nil {
			return line.String(), err
		}
	}
	return line.String(), errors.New("approval response is too long")
}

func codeCommandApprover(jsonOutput bool, in io.Reader, out io.Writer) func(context.Context, []string) (tools.CommandDecision, error) {
	if jsonOutput {
		return nil
	}
	return func(ctx context.Context, argv []string) (tools.CommandDecision, error) {
		select {
		case <-ctx.Done():
			return tools.CommandDeny, ctx.Err()
		default:
		}
		if _, err := fmt.Fprintf(out, "\nInternal Code command approval\n  argv: %s\nAllow this command? [y once/a always/N] ", strings.Join(argv, " ")); err != nil {
			return tools.CommandDeny, err
		}
		line, err := readApprovalLine(in)
		if errors.Is(err, io.EOF) && line == "" {
			return tools.CommandDeny, nil
		}
		if err != nil && !(errors.Is(err, io.EOF) && line != "") {
			return tools.CommandDeny, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return tools.CommandAllowOnce, nil
		case "a", "always":
			return tools.CommandAllowAlways, nil
		default:
			return tools.CommandDeny, nil
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
