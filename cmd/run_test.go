package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm/faketest"
	pawlog "github.com/gongahkia/paw/internal/log"
	"github.com/gongahkia/paw/internal/policy"
	"github.com/gongahkia/paw/internal/session"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestStageCommandsGolden(t *testing.T) {
	isolateEnv(t)

	t.Run("gather", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "notes.txt", "alpha\nbeta target\nomega\n")
		chdir(t, dir)

		out := executeRoot(t, append(configArgs(t), "gather", "--instruction", "target"), "")
		assertGoldenEnvelope(t, "gather", out)
	})

	t.Run("compress", func(t *testing.T) {
		env := envelope.NewEnvelope("task-compress", "target", "/work")
		env.Stage = "gather"
		env.Raw = &envelope.RawContext{
			Units: []envelope.RawUnit{{
				ID:        "u001",
				Kind:      "file_slice",
				Path:      "app.txt",
				StartLine: 1,
				EndLine:   1,
				Text:      "target line\n",
			}},
			TotalBytes: len("target line\n"),
		}

		out := executeRoot(t, append(configArgs(t), "compress", "--disable-compress"), marshalEnvelope(t, env))
		assertGoldenEnvelope(t, "compress", out)
	})

	t.Run("plan", func(t *testing.T) {
		server := faketest.NewServer()
		t.Cleanup(server.Close)
		configureBrain(t, server.URL)
		server.RespondOpenAI("Prior VerifyResult JSON", `{"done":false,"reasoning":"edit fixture","next_action":{"kind":"edit_file","description":"update file","target_path":"file.txt"}}`)

		env := digestEnvelope("task-plan", "/work")
		out := executeRoot(t, append(configArgs(t), "plan"), marshalEnvelope(t, env))
		assertGoldenEnvelope(t, "plan", out)
	})

	t.Run("edit", func(t *testing.T) {
		server := faketest.NewServer()
		t.Cleanup(server.Close)
		configureBrain(t, server.URL)
		server.RespondOpenAI("Target path", strings.Join([]string{
			"--- a/file.txt",
			"+++ b/file.txt",
			"@@ -1,3 +1,3 @@",
			" one",
			"-two",
			"+TWO",
			" three",
			"",
		}, "\n"))

		dir := t.TempDir()
		writeTestFile(t, dir, "file.txt", "one\ntwo\nthree\n")
		env := digestEnvelope("task-edit", dir)
		env.Plan = &envelope.Plan{
			Done:      false,
			Reasoning: "edit fixture",
			NextAction: &envelope.NextAction{
				Kind:        "edit_file",
				Description: "update file",
				TargetPath:  "file.txt",
			},
		}

		out := executeRoot(t, append(configArgs(t), "edit"), marshalEnvelope(t, env))
		assertGoldenEnvelope(t, "edit", out)
		if got := readTestFile(t, dir, "file.txt"); got != "one\nTWO\nthree\n" {
			t.Fatalf("patched file mismatch:\n%s", got)
		}
	})

	t.Run("verify", func(t *testing.T) {
		t.Setenv("PAW_VERIFY_CMD", "printf 'ok\\n'")
		env := envelope.NewEnvelope("task-verify", "target", t.TempDir())

		out := executeRoot(t, append(configArgs(t), "verify"), marshalEnvelope(t, env))
		assertGoldenEnvelope(t, "verify", out)
	})
}

func TestRunFailsFastOnMissingBrainKey(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	chdir(t, dir)
	configFile := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configFile, []byte(`
[brain]
transport = "openai"
base_url = "https://api.openai.com/v1"
model = "gpt-test"

[policy.provider]
allowed_transports = ["openai"]
allowed_base_urls = ["https://api.openai.com/v1"]
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, stderr, err := executeRootErr(t, []string{"--config", configFile, "run", "--raw-context", "--instruction", "target"}, "")
	if err == nil || !strings.Contains(err.Error(), "PAW_BRAIN_API_KEY") {
		t.Fatalf("expected PAW_BRAIN_API_KEY error, got err=%v stderr=%s", err, stderr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".paw")); !os.IsNotExist(statErr) {
		t.Fatalf("unexpected .paw dir after fail-fast: %v", statErr)
	}
}

func TestRunCreatesReviewSessionWithoutEditing(t *testing.T) {
	isolateEnv(t)
	server := faketest.NewServer()
	t.Cleanup(server.Close)
	configureBrain(t, server.URL)
	server.RespondOpenAI("Prior VerifyResult JSON", `{"done":false,"reasoning":"change Add","next_action":{"kind":"edit_file","description":"return a + b","target_path":"calc.go"}}`)

	dir := t.TempDir()
	chdir(t, dir)
	writeTestFile(t, dir, "calc.go", "package demo\n\nfunc Add(a, b int) int { return a - b }\n")
	stdout, _, err := executeRootErr(t, append(configArgs(t), "run", "--raw-context", "--quiet", "--instruction", "fix Add"), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var out envelope.Envelope
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode run output: %v\n%s", err, stdout)
	}
	if out.Stage != "plan" || out.Plan == nil || out.Patch != nil || out.Verify != nil {
		t.Fatalf("run output = %#v", out)
	}
	if got := readTestFile(t, dir, "calc.go"); !strings.Contains(got, "return a - b") {
		t.Fatalf("workspace changed:\n%s", got)
	}
	if requests := server.Requests(); len(requests) != 1 {
		t.Fatalf("model requests = %d", len(requests))
	}
	manifests, err := session.List(dir)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(manifests) != 1 || manifests[0].Status != session.StatusActive {
		t.Fatalf("sessions = %#v", manifests)
	}
	store, err := session.Open(dir, manifests[0].ID)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	events, err := store.Events()
	if err != nil {
		t.Fatalf("read session events: %v", err)
	}
	if len(events) != 2 || events[0].Type != "session.created" || events[1].Type != "plan.proposed" {
		t.Fatalf("session events = %#v", events)
	}
	var proposal struct {
		TaskID      string         `json:"task_id"`
		Instruction string         `json:"instruction"`
		Plan        *envelope.Plan `json:"plan"`
	}
	if err := json.Unmarshal(events[1].Data, &proposal); err != nil {
		t.Fatalf("decode proposed plan: %v", err)
	}
	if proposal.TaskID != out.TaskID || proposal.Instruction != "fix Add" || proposal.Plan == nil || proposal.Plan.NextAction == nil || proposal.Plan.NextAction.TargetPath != "calc.go" {
		t.Fatalf("proposal = %#v", proposal)
	}
}

func TestRunRecordsFailedReviewSession(t *testing.T) {
	isolateEnv(t)
	server := faketest.NewServer()
	t.Cleanup(server.Close)
	configureBrain(t, server.URL)
	server.Respond("brain planning stage", 500, `{"error":"plan unavailable"}`)

	dir := t.TempDir()
	chdir(t, dir)
	writeTestFile(t, dir, "notes.txt", "target\n")
	_, _, err := executeRootErr(t, append(configArgs(t), "run", "--raw-context", "--quiet", "--instruction", "target"), "")
	if err == nil {
		t.Fatal("run error = nil")
	}
	manifests, listErr := session.List(dir)
	if listErr != nil {
		t.Fatalf("list sessions: %v", listErr)
	}
	if len(manifests) != 1 || manifests[0].Status != session.StatusFailed {
		t.Fatalf("sessions = %#v", manifests)
	}
	store, openErr := session.Open(dir, manifests[0].ID)
	if openErr != nil {
		t.Fatalf("open session: %v", openErr)
	}
	events, eventsErr := store.Events()
	if eventsErr != nil {
		t.Fatalf("read session events: %v", eventsErr)
	}
	if len(events) != 2 || events[0].Type != "session.created" || events[1].Type != "plan.failed" {
		t.Fatalf("session events = %#v", events)
	}
}

func TestProviderClientsRejectUnallowlistedEndpoints(t *testing.T) {
	cfg := config.Defaults()
	cfg.Brain.Transport = "openai"
	cfg.Brain.BaseURL = "https://api.example.test/v1"
	if _, err := newBrainClient(cfg); !errors.Is(err, policy.ErrDenied) {
		t.Fatalf("brain allowlist error = %v", err)
	}
	cfg.Drone.Transport = "openai"
	cfg.Drone.BaseURL = "https://api.example.test/v1"
	if _, err := newDroneClient(cfg); !errors.Is(err, policy.ErrDenied) {
		t.Fatalf("drone allowlist error = %v", err)
	}
}

func TestRunVerboseMirrorsTraceToStderr(t *testing.T) {
	isolateEnv(t)

	run := func(verboseFlag bool) string {
		server := faketest.NewServer()
		t.Cleanup(server.Close)
		configureBrain(t, server.URL)
		server.RespondOpenAI("Prior VerifyResult JSON", `{"done":true,"reasoning":"done"}`)

		dir := t.TempDir()
		chdir(t, dir)
		writeTestFile(t, dir, "notes.txt", "target\n")

		args := configArgs(t)
		if verboseFlag {
			args = append(args, "--verbose")
		}
		args = append(args, "run", "--raw-context", "--instruction", "target")
		_, stderr, err := executeRootErr(t, args, "")
		if err != nil {
			t.Fatalf("run verbose=%v: %v stderr=%s", verboseFlag, err, stderr)
		}
		return stderr
	}

	if got := run(false); strings.Contains(got, "stage=gather") || !strings.Contains(got, "[gather]") {
		t.Fatalf("default stderr = %q", got)
	}
	got := run(true)
	for _, want := range []string{"[gather]", "stage=gather", "stage=plan", "dropped_items=0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("verbose stderr missing %q:\n%s", want, got)
		}
	}
}

func TestRunProgressAndQuiet(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	chdir(t, dir)
	writeTestFile(t, dir, "notes.txt", "target\n")

	run := func(extra ...string) (string, string) {
		server := faketest.NewServer()
		t.Cleanup(server.Close)
		configureBrain(t, server.URL)
		server.RespondOpenAI("Prior VerifyResult JSON", `{"done":true,"reasoning":"done"}`)

		args := append(configArgs(t), "run", "--raw-context", "--instruction", "target")
		args = append(args, extra...)
		stdout, stderr, err := executeRootErr(t, args, "")
		if err != nil {
			t.Fatalf("run %v: %v stderr=%s", extra, err, stderr)
		}
		return stdout, stderr
	}

	stdout, stderr := run()
	for _, want := range []string{"[gather] collected", "[compress] raw context", "[plan] done"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("progress stderr missing %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "\x1b[") {
		t.Fatalf("non-tty stderr contains ANSI escapes:\n%q", stderr)
	}

	quietStdout, quietStderr := run("--quiet")
	if quietStderr != "" {
		t.Fatalf("quiet stderr = %q", quietStderr)
	}
	if got, want := canonicalEnvelopeJSON(t, []byte(stdout), true), canonicalEnvelopeJSON(t, []byte(quietStdout), true); got != want {
		t.Fatalf("stdout changed with quiet\ngot:\n%s\nwant:\n%s", got, want)
	}
	_, quietVerboseStderr := run("--verbose", "--quiet")
	if quietVerboseStderr != "" {
		t.Fatalf("quiet verbose stderr = %q", quietVerboseStderr)
	}
}

func TestRunJSONLogFormat(t *testing.T) {
	isolateEnv(t)

	run := func(level string) (string, []map[string]any) {
		server := faketest.NewServer()
		t.Cleanup(server.Close)
		configureBrain(t, server.URL)
		server.RespondOpenAI("Prior VerifyResult JSON", `{"done":true,"reasoning":"done"}`)

		dir := t.TempDir()
		chdir(t, dir)
		writeTestFile(t, dir, "notes.txt", "target\n")

		args := append(configArgs(t), "--log-format", "json", "--log-level", level, "--verbose", "run", "--raw-context", "--instruction", "target")
		stdout, stderr, err := executeRootErr(t, args, "")
		if err != nil {
			t.Fatalf("run json logs: %v stderr=%s", err, stderr)
		}
		_ = canonicalEnvelopeJSON(t, []byte(stdout), true)
		return stderr, decodeLogLines(t, stderr)
	}

	infoStderr, infoRecords := run("info")
	if strings.Contains(infoStderr, "[gather]") {
		t.Fatalf("json stderr contains text progress:\n%s", infoStderr)
	}
	if !hasLog(infoRecords, "stage complete") {
		t.Fatalf("missing progress log:\n%s", infoStderr)
	}
	if hasLog(infoRecords, "stage trace") {
		t.Fatalf("debug trace visible at info level:\n%s", infoStderr)
	}

	debugStderr, debugRecords := run("debug")
	if !hasLog(debugRecords, "stage trace") {
		t.Fatalf("missing debug trace:\n%s", debugStderr)
	}
}

func executeRoot(t *testing.T, args []string, input string) string {
	t.Helper()
	resetCLIState(t)
	defer resetCLIState(t)

	var stdout, stderr bytes.Buffer
	rootCmd.SetArgs(args)
	rootCmd.SetIn(strings.NewReader(input))
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute %v: %v\nstderr:\n%s\nstdout:\n%s", args, err, stderr.String(), stdout.String())
	}
	return stdout.String()
}

func resetCLIState(t *testing.T) {
	t.Helper()
	configPath = ""
	traceFile = ""
	verbose = false
	noColor = false
	logFormat = "text"
	logLevel = "info"
	runInstruction = ""
	runInstructionFile = ""
	runMaxTurns = 0
	runRawContext = false
	runDisableCompress = false
	runDroneModel = ""
	runNoninteractive = false
	runExplain = false
	runQuiet = false
	initNonInteractive = false
	initLocation = ""
	initBrainTransport = ""
	initBrainModel = ""
	initBrainBaseURL = ""
	initBrainAPIKey = ""
	initDroneTransport = ""
	initDroneModel = ""
	initDroneBaseURL = ""
	initDroneAPIKey = ""
	doctorJSON = false
	doctorLint = false
	doctorDeep = false
	doctorFix = false
	doctorDryRun = false
	doctorYes = false
	doctorNonInteractive = false
	doctorSeverityMin = "info"
	doctorOnly = nil
	doctorSkip = nil
	doctorHistoryJSON = false
	doctorHistoryLimit = 20
	resumeMaxTurns = 0
	resumeRawContext = false
	resumeDisableCompress = false
	resumeDroneModel = ""
	resumeQuiet = false
	statsRate = ""
	watchMarkers = "ai:"
	watchDebounce = 200 * time.Millisecond
	watchMaxTurns = 0
	watchRawContext = false
	watchDisableCompress = false
	watchDroneModel = ""
	watchQuiet = false
	gatherInstruction = ""
	compressDisableCompress = false
	compressDroneModel = ""
	mcpServeDisableCompress = false
	modelsListQuery = ""
	modelsListProvider = ""
	sessionJSON = false
	rootCmd.SetArgs(nil)
	rootCmd.SetIn(strings.NewReader(""))
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	resetCommandFlags(t, rootCmd)
	_, _ = pawlog.Install(pawlog.Config{Writer: io.Discard})
}

func resetCommandFlags(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	resetFlagSet(t, cmd.Flags())
	resetFlagSet(t, cmd.PersistentFlags())
	for _, child := range cmd.Commands() {
		resetCommandFlags(t, child)
	}
}

func resetFlagSet(t *testing.T, flags *pflag.FlagSet) {
	t.Helper()
	flags.VisitAll(func(flag *pflag.Flag) {
		if err := flag.Value.Set(flag.DefValue); err != nil {
			t.Fatalf("reset flag %s: %v", flag.Name, err)
		}
		flag.Changed = false
	})
}

func isolateEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PAW_BRAIN_TRANSPORT",
		"PAW_BRAIN_BASE_URL",
		"PAW_BRAIN_API_KEY",
		"PAW_BRAIN_PROVIDER",
		"PAW_BRAIN_MODEL",
		"PAW_DRONE_TRANSPORT",
		"PAW_DRONE_BASE_URL",
		"PAW_DRONE_API_KEY",
		"PAW_DRONE_PROVIDER",
		"PAW_DRONE_MODEL",
		"PAW_MAX_TURNS",
		"PAW_MAX_BRAIN_TOKENS",
		"PAW_CALL_TIMEOUT",
		"PAW_OLLAMA_AUTO_PULL",
		"PAW_TLS_CA_FILE",
		"PAW_INSECURE_SKIP_TLS_VERIFY",
		"PAW_GATHER_MAX_DEPTH",
		"PAW_GATHER_MAX_FILE_BYTES",
		"PAW_VERIFY_CMD",
		"PAW_VERIFY_TIMEOUT",
		"PAW_TRACE_MODE",
		"PAW_BENCH_TRACE_MODE",
		"PAW_POLICY_ALLOWED_TRANSPORTS",
		"PAW_POLICY_ALLOWED_BASE_URLS",
		"PAW_POLICY_ALLOW_LOOPBACK",
		"PAW_POLICY_BLOCK_SECRETS",
		"PAW_POLICY_AUTO_APPROVE",
	} {
		t.Setenv(key, "")
	}
}

func configureBrain(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("PAW_BRAIN_TRANSPORT", "openai")
	t.Setenv("PAW_BRAIN_BASE_URL", baseURL)
	t.Setenv("PAW_BRAIN_API_KEY", "test-key")
	t.Setenv("PAW_BRAIN_MODEL", "test-brain")
	t.Setenv("PAW_POLICY_ALLOWED_TRANSPORTS", "ollama,openai")
}

func configArgs(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return []string{"--config", path}
}

func digestEnvelope(taskID, cwd string) *envelope.Envelope {
	env := envelope.NewEnvelope(taskID, "target", cwd)
	env.Digest = &envelope.ContextDigest{
		Summary: "fixture digest",
		Items: []envelope.DigestItem{{
			UnitID:    "u001",
			Path:      "file.txt",
			Relevance: 100,
			Spans: []envelope.DigestSpan{{
				StartLine: 1,
				EndLine:   3,
				Quote:     "one\ntwo\nthree\n",
			}},
		}},
	}
	return env
}

func marshalEnvelope(t *testing.T, env *envelope.Envelope) string {
	t.Helper()
	var b bytes.Buffer
	if err := env.Marshal(&b); err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return b.String()
}

func assertGoldenEnvelope(t *testing.T, name, got string) {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller failed")
	}
	wantPath := filepath.Join(filepath.Dir(source), "testdata", "golden", name+".json")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	gotJSON := canonicalEnvelopeJSON(t, []byte(got), true)
	wantJSON := canonicalEnvelopeJSON(t, want, false)
	if gotJSON != wantJSON {
		t.Fatalf("%s golden mismatch\ngot:\n%s\nwant:\n%s", name, gotJSON, wantJSON)
	}
}

func canonicalEnvelopeJSON(t *testing.T, data []byte, normalize bool) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode json: %v\n%s", err, data)
	}
	if normalize {
		normalizeEnvelope(v)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("canonical json: %v", err)
	}
	return string(out) + "\n"
}

func normalizeEnvelope(v any) {
	env, ok := v.(map[string]any)
	if !ok {
		return
	}
	if _, ok := env["cwd"]; ok {
		env["cwd"] = "<cwd>"
	}
	if _, ok := env["task_id"]; ok {
		env["task_id"] = "<task>"
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func writeTestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func readTestFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(b)
}

func decodeLogLines(t *testing.T, stderr string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode log line: %v\n%s", err, line)
		}
		records = append(records, record)
	}
	return records
}

func hasLog(records []map[string]any, msg string) bool {
	for _, record := range records {
		if record["msg"] == msg {
			return true
		}
	}
	return false
}
