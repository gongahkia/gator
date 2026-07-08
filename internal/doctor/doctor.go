package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/verify"
)

type EndpointHealth interface {
	Check(context.Context, llm.EndpointConfig) llm.HealthReport
}

type Options struct {
	ConfigPath     string
	CWD            string
	Version        string
	Deep           bool
	Fix            bool
	DryRun         bool
	Yes            bool
	NonInteractive bool
	SeverityMin    Severity
	Only           []string
	Skip           []string
	HealthChecker  EndpointHealth
	Stdin          io.Reader
	PromptWriter   io.Writer
	Now            func() time.Time
}

func Run(ctx context.Context, opts Options) Report {
	r := runner{
		opts:          normalizeOptions(opts),
		only:          selectorSet(opts.Only),
		skip:          selectorSet(opts.Skip),
		healthChecker: opts.HealthChecker,
	}
	if r.healthChecker == nil {
		r.healthChecker = llm.EndpointHealthChecker{}
	}
	r.report = Report{
		SchemaVersion: 1,
		Version:       r.opts.Version,
		CWD:           r.opts.CWD,
	}
	r.gitRoot = findGitRoot(r.opts.CWD)
	cfg, loadErr := r.loadConfig()
	if r.shouldRunSection("system") {
		r.checkSystem()
	}
	if r.shouldRunSection("repo") {
		r.checkRepo(ctx)
	}
	if r.shouldRunSection("config") {
		r.checkConfig(loadErr, cfg)
	}
	if r.shouldRunSection("verify") {
		r.checkVerify(ctx, cfg)
	}
	if r.shouldRunSection("models") {
		r.checkModels(ctx, cfg)
	}
	if r.shouldRunSection("trace") {
		r.checkTrace()
	}
	if r.shouldRunSection("onboarding") {
		r.checkOnboarding(cfg)
	}
	r.report.Summary = summarize(r.report.Findings)
	return r.report
}

func (r Report) HasIssues(min Severity) bool {
	for _, f := range r.Findings {
		if severityRank(f.Severity) >= severityRank(min) && (f.Status == StatusWarn || f.Status == StatusFail) {
			return true
		}
	}
	return false
}

type runner struct {
	opts          Options
	only          map[string]bool
	skip          map[string]bool
	healthChecker EndpointHealth
	report        Report
	configSources []configSource
	gitRoot       string
}

type configSource struct {
	Name     string
	Path     string
	Exists   bool
	Explicit bool
}

func normalizeOptions(opts Options) Options {
	if opts.CWD == "" {
		if cwd, err := os.Getwd(); err == nil {
			opts.CWD = cwd
		}
	}
	if opts.CWD == "" {
		opts.CWD = "."
	}
	if opts.SeverityMin == "" {
		opts.SeverityMin = SeverityInfo
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.PromptWriter == nil {
		opts.PromptWriter = io.Discard
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return opts
}

func (r *runner) loadConfig() (config.Config, error) {
	r.configSources = discoverConfigSources(r.opts.CWD, r.opts.ConfigPath)
	cfg, err := config.Load(r.opts.ConfigPath)
	if err == nil {
		return cfg, nil
	}
	return config.Defaults(), err
}

func (r *runner) checkSystem() {
	r.add(Finding{ID: "system.runtime", Section: "system", Severity: SeverityInfo, Status: StatusOK, Message: "runtime detected", Detail: runtime.GOOS + "/" + runtime.GOARCH, Metadata: map[string]string{"go": runtime.Version()}})
	r.add(Finding{ID: "system.cwd", Section: "system", Severity: SeverityInfo, Status: StatusOK, Message: "current directory", Path: r.opts.CWD})
	if os.Getenv("PATH") == "" {
		r.add(Finding{ID: "system.path", Section: "system", Severity: SeverityError, Status: StatusFail, Message: "PATH is empty", Fix: "set PATH before running paw"})
	} else {
		r.add(Finding{ID: "system.path", Section: "system", Severity: SeverityInfo, Status: StatusOK, Message: "PATH is set"})
	}
	r.checkPawDir()
}

func (r *runner) checkPawDir() {
	path := filepath.Join(r.opts.CWD, ".paw")
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			r.add(Finding{ID: "system.paw_dir", Section: "system", Severity: SeverityError, Status: StatusFail, Message: ".paw exists but is not a directory", Path: path, Fix: "move the file and create .paw"})
			return
		}
		r.add(Finding{ID: "system.paw_dir", Section: "system", Severity: SeverityInfo, Status: StatusOK, Message: ".paw directory exists", Path: path})
		return
	}
	if !errors.Is(err, os.ErrNotExist) {
		r.add(Finding{ID: "system.paw_dir", Section: "system", Severity: SeverityError, Status: StatusFail, Message: "cannot inspect .paw directory", Detail: err.Error(), Path: path})
		return
	}
	if r.opts.Fix {
		if r.opts.DryRun {
			r.add(Finding{ID: "system.paw_dir", Section: "system", Severity: SeverityInfo, Status: StatusPlanned, Message: "would create .paw directory", Path: path})
			return
		}
		allowed, detail := r.confirmRepair("create .paw directory", path)
		if !allowed {
			r.add(Finding{ID: "system.paw_dir", Section: "system", Severity: SeverityWarning, Status: StatusSkipped, Message: "skipped .paw directory repair", Detail: detail, Path: path, Fix: "rerun with `paw doctor --fix --yes`"})
			return
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			r.logOperation("create .paw directory", "fail", path, "", err)
			r.add(Finding{ID: "system.paw_dir", Section: "system", Severity: SeverityError, Status: StatusFail, Message: "failed to create .paw directory", Detail: err.Error(), Path: path})
			return
		}
		r.logOperation("create .paw directory", "ok", path, "", nil)
		r.add(Finding{ID: "system.paw_dir", Section: "system", Severity: SeverityInfo, Status: StatusFixed, Message: "created .paw directory", Path: path})
		return
	}
	r.add(Finding{ID: "system.paw_dir", Section: "system", Severity: SeverityWarning, Status: StatusWarn, Message: ".paw directory is missing", Path: path, Fix: "run `paw doctor --fix`"})
}

func (r *runner) checkRepo(ctx context.Context) {
	root := r.gitRoot
	r.gitRoot = root
	if root == "" {
		r.add(Finding{ID: "repo.git", Section: "repo", Severity: SeverityWarning, Status: StatusWarn, Message: "not inside a git repo", Fix: "run paw from the repository root"})
		return
	}
	r.add(Finding{ID: "repo.git", Section: "repo", Severity: SeverityInfo, Status: StatusOK, Message: "git repo detected", Path: root})
	if out, err := runCommand(ctx, root, 2*time.Second, "git", "status", "--short"); err == nil {
		lines := nonemptyLines(out)
		if len(lines) == 0 {
			r.add(Finding{ID: "repo.dirty", Section: "repo", Severity: SeverityInfo, Status: StatusOK, Message: "git worktree clean"})
		} else {
			r.add(Finding{ID: "repo.dirty", Section: "repo", Severity: SeverityWarning, Status: StatusWarn, Message: "git worktree has local changes", Detail: fmt.Sprintf("%d changed paths", len(lines)), Command: "git status --short"})
		}
	} else {
		r.add(Finding{ID: "repo.dirty", Section: "repo", Severity: SeverityWarning, Status: StatusWarn, Message: "cannot check git status", Detail: err.Error(), Command: "git status --short"})
	}
	if _, err := runCommand(ctx, root, 2*time.Second, "git", "check-ignore", "-q", ".paw"); err == nil {
		r.add(Finding{ID: "repo.paw_ignored", Section: "repo", Severity: SeverityInfo, Status: StatusOK, Message: ".paw is ignored by git"})
	} else {
		r.add(Finding{ID: "repo.paw_ignored", Section: "repo", Severity: SeverityWarning, Status: StatusWarn, Message: ".paw may not be ignored", Detail: ".paw traces can contain raw repository context", Fix: "add `.paw/` to .gitignore"})
	}
}

func (r *runner) checkConfig(loadErr error, cfg config.Config) {
	for _, source := range r.configSources {
		status := StatusOK
		severity := SeverityInfo
		message := source.Name + " config found"
		if !source.Exists {
			message = source.Name + " config missing"
			status = StatusSkipped
			if source.Explicit {
				status = StatusFail
				severity = SeverityError
			}
		}
		r.add(Finding{ID: "config.source." + source.Name, Section: "config", Severity: severity, Status: status, Message: message, Path: source.Path})
	}
	if loadErr != nil {
		f := Finding{ID: "config.load", Section: "config", Severity: SeverityError, Status: StatusFail, Message: "config load failed", Detail: loadErr.Error(), Fix: "repair or remove the invalid config"}
		if r.opts.Fix {
			path := repairConfigPath(r.opts, r.configSources, r.gitRoot)
			if path != "" {
				if r.opts.DryRun {
					f.Status = StatusPlanned
					f.Severity = SeverityInfo
					f.Message = "would repair config by writing defaults"
					f.Path = path
					f.Fix = ""
				} else if allowed, detail := r.confirmRepair("repair config", path); !allowed {
					f.Status = StatusSkipped
					f.Severity = SeverityWarning
					f.Message = "skipped config repair"
					f.Detail = detail
					f.Path = path
					f.Fix = "rerun with `paw doctor --fix --yes`"
				} else if err := backupAndSaveConfig(path, cfg); err != nil {
					r.logOperation("repair config", "fail", path, "", err)
					f.Detail = f.Detail + "; repair failed: " + err.Error()
				} else {
					r.logOperation("repair config", "ok", path, "backup written if config existed", nil)
					f.Status = StatusFixed
					f.Severity = SeverityInfo
					f.Message = "repaired config by writing defaults"
					f.Path = path
					f.Fix = ""
				}
			}
		}
		r.add(f)
	} else {
		r.add(Finding{ID: "config.load", Section: "config", Severity: SeverityInfo, Status: StatusOK, Message: "config loaded"})
	}
	r.checkMissingConfig(cfg)
	r.checkEnvOverrides()
	if cfg.TLS.InsecureSkipVerify {
		r.add(Finding{ID: "config.tls_insecure", Section: "config", Severity: SeverityWarning, Status: StatusWarn, Message: "TLS verification is disabled", Fix: "unset PAW_INSECURE_SKIP_TLS_VERIFY or remove tls.insecure_skip_verify"})
	}
}

func (r *runner) checkMissingConfig(cfg config.Config) {
	for _, source := range r.configSources {
		if source.Exists {
			return
		}
	}
	path := repairConfigPath(r.opts, r.configSources, r.gitRoot)
	if r.opts.Fix {
		if r.opts.DryRun {
			r.add(Finding{ID: "config.bootstrap", Section: "config", Severity: SeverityInfo, Status: StatusPlanned, Message: "would create config", Path: path})
			return
		}
		allowed, detail := r.confirmRepair("create config", path)
		if !allowed {
			r.add(Finding{ID: "config.bootstrap", Section: "config", Severity: SeverityWarning, Status: StatusSkipped, Message: "skipped config creation", Detail: detail, Path: path, Fix: "rerun with `paw doctor --fix --yes`"})
			return
		}
		if err := saveConfigWithMode(path, cfg, 0o600); err != nil {
			r.logOperation("create config", "fail", path, "", err)
			r.add(Finding{ID: "config.bootstrap", Section: "config", Severity: SeverityError, Status: StatusFail, Message: "failed to create config", Detail: err.Error(), Path: path})
			return
		}
		r.logOperation("create config", "ok", path, "mode=0600", nil)
		r.add(Finding{ID: "config.bootstrap", Section: "config", Severity: SeverityInfo, Status: StatusFixed, Message: "created config", Path: path})
		return
	}
	r.add(Finding{ID: "config.bootstrap", Section: "config", Severity: SeverityWarning, Status: StatusWarn, Message: "no config file found; using built-in defaults", Path: path, Fix: "run `paw doctor --fix` or `paw init`"})
}

func (r *runner) checkEnvOverrides() {
	var active []string
	for _, key := range pawEnvKeys() {
		if os.Getenv(key) != "" {
			active = append(active, key)
		}
	}
	if len(active) == 0 {
		r.add(Finding{ID: "config.env", Section: "config", Severity: SeverityInfo, Status: StatusOK, Message: "no PAW_* config overrides detected"})
		return
	}
	sort.Strings(active)
	r.add(Finding{ID: "config.env", Section: "config", Severity: SeverityInfo, Status: StatusOK, Message: "PAW_* config overrides active", Detail: strings.Join(active, ", ")})
}

func (r *runner) checkVerify(ctx context.Context, cfg config.Config) {
	cmdText, source := resolvedVerifyCommand(cfg, r.opts.CWD)
	r.add(Finding{ID: "verify.command", Section: "verify", Severity: SeverityInfo, Status: StatusOK, Message: "verify command resolved", Detail: source, Command: cmdText})
	if cfg.Verify.Timeout <= 0 {
		r.add(Finding{ID: "verify.timeout", Section: "verify", Severity: SeverityWarning, Status: StatusWarn, Message: "verify timeout is disabled", Fix: "set PAW_VERIFY_TIMEOUT or [verify].timeout"})
	} else {
		r.add(Finding{ID: "verify.timeout", Section: "verify", Severity: SeverityInfo, Status: StatusOK, Message: "verify timeout set", Detail: cfg.Verify.Timeout.String()})
	}
	if bin := firstCommandWord(cmdText); bin != "" && bin != "true" {
		if _, err := exec.LookPath(bin); err != nil {
			r.add(Finding{ID: "verify.binary", Section: "verify", Severity: SeverityError, Status: StatusFail, Message: "verify command binary missing", Detail: err.Error(), Command: bin, Fix: "install " + bin + " or set PAW_VERIFY_CMD"})
		} else {
			r.add(Finding{ID: "verify.binary", Section: "verify", Severity: SeverityInfo, Status: StatusOK, Message: "verify command binary found", Command: bin})
		}
	}
	if !r.opts.Deep {
		r.add(Finding{ID: "verify.dry_run", Section: "verify", Severity: SeverityInfo, Status: StatusSkipped, Message: "verify dry-run skipped", Fix: "run `paw doctor --deep`"})
		return
	}
	start := time.Now()
	out, err := runShell(ctx, r.opts.CWD, cfg.Verify.Timeout, cmdText)
	elapsed := time.Since(start)
	if err != nil {
		r.add(Finding{ID: "verify.dry_run", Section: "verify", Severity: SeverityError, Status: StatusFail, Message: "verify command failed", Detail: tail(out, 600), Command: cmdText, Metadata: elapsedMetadata(elapsed, cfg.Verify.Timeout)})
		return
	}
	r.add(Finding{ID: "verify.dry_run", Section: "verify", Severity: SeverityInfo, Status: StatusOK, Message: "verify command passed", Command: cmdText, Metadata: elapsedMetadata(elapsed, cfg.Verify.Timeout)})
}

func (r *runner) checkModels(ctx context.Context, cfg config.Config) {
	checker := r.healthChecker
	if concrete, ok := checker.(llm.EndpointHealthChecker); ok {
		concrete.SchemaSmokeOllama = r.opts.Deep
		concrete.AutoPullOllama = cfg.OllamaAutoPull || r.opts.Fix && r.opts.Yes && !r.opts.NonInteractive
		checker = concrete
	}
	for _, endpoint := range []struct {
		name string
		cfg  config.EndpointConfig
	}{
		{name: "brain", cfg: cfg.Brain},
		{name: "drone", cfg: cfg.Drone},
	} {
		start := time.Now()
		report := checker.Check(ctx, llm.EndpointConfig{
			Transport: endpoint.cfg.Transport,
			BaseURL:   endpoint.cfg.BaseURL,
			APIKey:    endpoint.cfg.APIKey,
			Provider:  endpoint.cfg.Provider,
			Model:     endpoint.cfg.Model,
		})
		r.add(Finding{ID: "models." + endpoint.name, Section: "models", Severity: SeverityInfo, Status: StatusOK, Message: endpoint.name + " endpoint", Detail: endpointDetail(report), Metadata: elapsedMetadata(time.Since(start), 0)})
		for _, check := range report.Checks {
			r.add(healthFinding(endpoint.name, check))
		}
		if strings.EqualFold(endpoint.cfg.Transport, "ollama") {
			r.add(ollamaFitFinding(r.opts.CWD, endpoint.name, endpoint.cfg.Model))
		}
		if r.opts.Deep && (strings.EqualFold(endpoint.cfg.Transport, "openai") || strings.EqualFold(endpoint.cfg.Transport, "anthropic")) {
			r.add(apiSmokeFinding(endpoint.name, endpoint.cfg.Transport, report))
		}
	}
}

func (r *runner) checkTrace() {
	mode := strings.TrimSpace(os.Getenv("PAW_TRACE_MODE"))
	if mode == "" || mode == "full" {
		r.add(Finding{ID: "trace.mode", Section: "trace", Severity: SeverityWarning, Status: StatusWarn, Message: "full traces can include raw repository text", Detail: defaultString(mode, "full(default)"), Fix: "set PAW_TRACE_MODE=compact for shareable traces"})
	} else {
		r.add(Finding{ID: "trace.mode", Section: "trace", Severity: SeverityInfo, Status: StatusOK, Message: "trace mode configured", Detail: mode})
	}
	if r.gitRoot == "" {
		r.add(Finding{ID: "trace.gitignore", Section: "trace", Severity: SeverityWarning, Status: StatusSkipped, Message: ".paw ignore check skipped outside git repo"})
		return
	}
	if _, err := runCommand(context.Background(), r.gitRoot, 2*time.Second, "git", "check-ignore", "-q", ".paw"); err == nil {
		r.add(Finding{ID: "trace.gitignore", Section: "trace", Severity: SeverityInfo, Status: StatusOK, Message: ".paw traces are ignored by git"})
		return
	}
	r.add(Finding{ID: "trace.gitignore", Section: "trace", Severity: SeverityWarning, Status: StatusWarn, Message: ".paw traces may be tracked", Fix: "add `.paw/` to .gitignore"})
}

func (r *runner) checkOnboarding(cfg config.Config) {
	switch {
	case cfg.Brain.Transport == "ollama" || cfg.Drone.Transport == "ollama":
		r.add(Finding{ID: "onboarding.next", Section: "onboarding", Severity: SeverityInfo, Status: StatusOK, Message: "next local setup command", Command: "ollama serve && ollama pull " + cfg.Brain.Model + " && ollama pull " + cfg.Drone.Model})
	case cfg.Brain.APIKey == "" && (cfg.Brain.Transport == "openai" || cfg.Brain.Transport == "anthropic"):
		r.add(Finding{ID: "onboarding.next", Section: "onboarding", Severity: SeverityWarning, Status: StatusWarn, Message: "brain API key missing", Fix: "export PAW_BRAIN_API_KEY"})
	default:
		r.add(Finding{ID: "onboarding.next", Section: "onboarding", Severity: SeverityInfo, Status: StatusOK, Message: "run the pipeline explanation", Command: "paw run --explain"})
	}
}

func (r *runner) add(f Finding) {
	if f.ID == "" {
		return
	}
	if severityRank(f.Severity) < severityRank(r.opts.SeverityMin) {
		return
	}
	if len(r.only) > 0 && !selectorMatches(r.only, f.ID, f.Section) {
		return
	}
	if selectorMatches(r.skip, f.ID, f.Section) {
		return
	}
	r.report.Findings = append(r.report.Findings, f)
}

func (r *runner) shouldRunSection(section string) bool {
	if selectorMatches(r.skip, section, section) {
		return false
	}
	if len(r.only) == 0 {
		return true
	}
	for selector := range r.only {
		if selector == section || strings.HasPrefix(selector, section+".") || strings.HasSuffix(selector, "*") && strings.HasPrefix(section+".", strings.TrimSuffix(selector, "*")) {
			return true
		}
	}
	return false
}

func healthFinding(name string, check llm.HealthCheck) Finding {
	severity := SeverityInfo
	status := StatusOK
	switch check.Status {
	case llm.HealthFail:
		severity = SeverityError
		status = StatusFail
	case llm.HealthUnknown:
		severity = SeverityWarning
		status = StatusWarn
	}
	id := "models." + name + "." + check.Name
	return Finding{ID: id, Section: "models", Severity: severity, Status: status, Message: name + " " + check.Name, Detail: check.Detail, Fix: check.Action}
}

func apiSmokeFinding(name, transport string, report llm.HealthReport) Finding {
	for _, check := range report.Checks {
		if (check.Name == "auth" || check.Name == "running") && check.Status == llm.HealthFail {
			return Finding{ID: "models." + name + ".api_smoke", Section: "models", Severity: SeverityError, Status: StatusFail, Message: name + " API smoke failed", Detail: check.Detail, Fix: check.Action}
		}
	}
	return Finding{ID: "models." + name + ".api_smoke", Section: "models", Severity: SeverityInfo, Status: StatusOK, Message: name + " API smoke passed", Detail: transport + " metadata endpoint responded"}
}

func ollamaFitFinding(cwd, name, model string) Finding {
	estimate := estimatedModelBytes(model)
	if estimate == 0 {
		return Finding{ID: "models." + name + ".fit", Section: "models", Severity: SeverityWarning, Status: StatusSkipped, Message: name + " local fit estimate unavailable", Detail: model}
	}
	mem := physicalMemoryBytes()
	disk := diskFreeBytes(cwd)
	meta := map[string]string{
		"model":           model,
		"estimated_bytes": strconv.FormatUint(estimate, 10),
	}
	if mem > 0 {
		meta["memory_bytes"] = strconv.FormatUint(mem, 10)
	}
	if disk > 0 {
		meta["disk_free_bytes"] = strconv.FormatUint(disk, 10)
	}
	detail := "estimated_model=" + humanBytes(estimate)
	if mem > 0 {
		detail += " memory=" + humanBytes(mem)
	}
	if disk > 0 {
		detail += " disk_free=" + humanBytes(disk)
	}
	if mem > 0 && mem < estimate+estimate/4 {
		return Finding{ID: "models." + name + ".fit", Section: "models", Severity: SeverityWarning, Status: StatusWarn, Message: name + " model may not fit local memory", Detail: detail, Metadata: meta, Fix: "choose a smaller model or API brain"}
	}
	if disk > 0 && disk < estimate {
		return Finding{ID: "models." + name + ".fit", Section: "models", Severity: SeverityWarning, Status: StatusWarn, Message: name + " model may not fit free disk", Detail: detail, Metadata: meta, Fix: "free disk or choose a smaller model"}
	}
	return Finding{ID: "models." + name + ".fit", Section: "models", Severity: SeverityInfo, Status: StatusOK, Message: name + " local fit estimate passed", Detail: detail, Metadata: meta}
}

func estimatedModelBytes(model string) uint64 {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "qwen3-coder-next"):
		return 52 << 30
	case strings.Contains(lower, "70b"):
		return 45 << 30
	case strings.Contains(lower, "30b"):
		return 24 << 30
	case strings.Contains(lower, "20b"):
		return 14 << 30
	case strings.Contains(lower, "14b"):
		return 10 << 30
	case strings.Contains(lower, "8b"):
		return 6 << 30
	case strings.Contains(lower, "7b"):
		return 5 << 30
	default:
		return 0
	}
}

func physicalMemoryBytes() uint64 {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		n, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		return n
	case "linux":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				n, _ := strconv.ParseUint(fields[1], 10, 64)
				return n * 1024
			}
		}
	}
	return 0
}

func diskFreeBytes(cwd string) uint64 {
	out, err := exec.Command("df", "-k", cwd).Output()
	if err != nil {
		return 0
	}
	lines := nonemptyLines(string(out))
	if len(lines) < 2 {
		return 0
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[3], 10, 64)
	return n * 1024
}

func humanBytes(n uint64) string {
	const gib = 1 << 30
	if n >= gib {
		return fmt.Sprintf("%.1fGiB", float64(n)/gib)
	}
	const mib = 1 << 20
	if n >= mib {
		return fmt.Sprintf("%.1fMiB", float64(n)/mib)
	}
	return fmt.Sprintf("%dB", n)
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func endpointDetail(report llm.HealthReport) string {
	var parts []string
	if report.Transport != "" {
		parts = append(parts, "transport="+report.Transport)
	}
	if report.Model != "" {
		parts = append(parts, "model="+report.Model)
	}
	if report.BaseURL != "" {
		parts = append(parts, "base_url="+report.BaseURL)
	}
	return strings.Join(parts, " ")
}

func elapsedMetadata(elapsed time.Duration, timeout time.Duration) map[string]string {
	meta := map[string]string{"elapsed_ms": strconv.FormatInt(elapsed.Milliseconds(), 10)}
	if timeout > 0 {
		meta["timeout"] = timeout.String()
	}
	return meta
}

func summarize(findings []Finding) Summary {
	var s Summary
	for _, f := range findings {
		switch f.Status {
		case StatusFixed:
			s.Fixed++
		case StatusPlanned:
			s.Planned++
		case StatusSkipped:
			s.Skipped++
		case StatusFail:
			s.Error++
		case StatusWarn:
			s.Warning++
		default:
			s.OK++
		}
	}
	return s
}

func severityRank(s Severity) int {
	switch s {
	case SeverityError:
		return 2
	case SeverityWarning:
		return 1
	default:
		return 0
	}
}

func ParseSeverity(raw string) (Severity, error) {
	switch Severity(strings.ToLower(strings.TrimSpace(raw))) {
	case SeverityInfo:
		return SeverityInfo, nil
	case SeverityWarning:
		return SeverityWarning, nil
	case SeverityError:
		return SeverityError, nil
	default:
		return "", fmt.Errorf("unsupported severity %q", raw)
	}
}

func Explanation(id string) (string, bool) {
	explanations := map[string]string{
		"system.paw_dir":       ".paw stores traces, doctor logs, and repo-local config. Repair creates the directory only.",
		"config.bootstrap":     "Paw can run on defaults, but a config file makes setup explicit. Repair writes a repo config inside .paw when running in a git repo.",
		"config.load":          "Invalid config prevents Paw from knowing model, transport, and verify settings. Repair backs up the current file and writes defaults.",
		"repo.paw_ignored":     ".paw traces may include raw repository context. Add .paw/ to .gitignore before sharing a repo.",
		"trace.mode":           "Full traces are best for resume/debugging but can include source text. Use PAW_TRACE_MODE=compact for shareable artifacts.",
		"verify.dry_run":       "Deep doctor can run the resolved verify command before an agent task, catching missing test tools early.",
		"models.brain.running": "The configured brain transport is unavailable. For local Ollama, start it with ollama serve before running Paw.",
		"models.drone.running": "The configured drone transport is unavailable. For local Ollama, start it with ollama serve before running Paw.",
		"models.brain.fit":     "Local model fit is an estimate from model naming, machine memory, and free disk. Treat warnings as capacity checks, not exact limits.",
		"models.drone.fit":     "Local model fit is an estimate from model naming, machine memory, and free disk. Treat warnings as capacity checks, not exact limits.",
	}
	text, ok := explanations[id]
	return text, ok
}

func selectorSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out[part] = true
			}
		}
	}
	return out
}

func selectorMatches(selectors map[string]bool, id, section string) bool {
	if len(selectors) == 0 {
		return false
	}
	if selectors[id] || selectors[section] {
		return true
	}
	for selector := range selectors {
		if strings.HasSuffix(selector, "*") && strings.HasPrefix(id, strings.TrimSuffix(selector, "*")) {
			return true
		}
	}
	return false
}

func discoverConfigSources(cwd, explicit string) []configSource {
	var sources []configSource
	if user := config.DefaultPath(); user != "" {
		sources = append(sources, statConfigSource("user", user, false))
	}
	if repo := discoverRepoConfig(cwd); repo != "" {
		sources = append(sources, statConfigSource("repo", repo, false))
	}
	if explicit != "" {
		sources = append(sources, statConfigSource("explicit", explicit, true))
	}
	return sources
}

func statConfigSource(name, path string, explicit bool) configSource {
	_, err := os.Stat(path)
	return configSource{Name: name, Path: path, Exists: err == nil, Explicit: explicit}
}

func discoverRepoConfig(cwd string) string {
	dir := cleanAbs(cwd)
	home := cleanAbs(os.Getenv("HOME"))
	for {
		candidate := filepath.Join(dir, ".paw", "config.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if fileExists(filepath.Join(dir, ".git")) || samePath(dir, home) {
			return filepath.Join(dir, ".paw", "config.toml")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func findGitRoot(cwd string) string {
	dir := cleanAbs(cwd)
	for {
		if fileExists(filepath.Join(dir, ".git")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func samePath(a, b string) bool {
	return b != "" && filepath.Clean(a) == filepath.Clean(b)
}

func cleanAbs(path string) string {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return filepath.Clean(abs)
}

func pawEnvKeys() []string {
	return []string{
		"PAW_BRAIN_TRANSPORT", "PAW_BRAIN_BASE_URL", "PAW_BRAIN_API_KEY", "PAW_BRAIN_PROVIDER", "PAW_BRAIN_MODEL",
		"PAW_DRONE_TRANSPORT", "PAW_DRONE_BASE_URL", "PAW_DRONE_API_KEY", "PAW_DRONE_PROVIDER", "PAW_DRONE_MODEL",
		"PAW_MAX_TURNS", "PAW_MAX_BRAIN_TOKENS", "PAW_CALL_TIMEOUT", "PAW_OLLAMA_AUTO_PULL",
		"PAW_TLS_CA_FILE", "PAW_INSECURE_SKIP_TLS_VERIFY", "PAW_GATHER_MAX_DEPTH", "PAW_GATHER_MAX_FILE_BYTES",
		"PAW_VERIFY_CMD", "PAW_VERIFY_TIMEOUT", "PAW_TRACE_MODE",
	}
}

func resolvedVerifyCommand(cfg config.Config, cwd string) (string, string) {
	if env := os.Getenv("PAW_VERIFY_CMD"); env != "" {
		return env, "PAW_VERIFY_CMD"
	}
	if cfg.Verify.Command != "" {
		return cfg.Verify.Command, "[verify].command"
	}
	return verify.ResolveCommand(cwd), "auto"
}

func firstCommandWord(cmdText string) string {
	fields := strings.Fields(cmdText)
	if len(fields) == 0 {
		return ""
	}
	switch fields[0] {
	case "env", "time", "command":
		if len(fields) > 1 {
			return fields[1]
		}
	}
	return strings.Trim(fields[0], `"'`)
}

func runShell(ctx context.Context, cwd string, timeout time.Duration, command string) (string, error) {
	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(runCtx, "sh", "-c", command)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String()
	if stderr.Len() > 0 {
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += stderr.String()
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("timed out after %s", timeout)
	}
	return out, err
}

func runCommand(ctx context.Context, cwd string, timeout time.Duration, command string, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command, args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String()
	if stderr.Len() > 0 {
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += stderr.String()
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("%s timed out", command)
	}
	return out, err
}

func nonemptyLines(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func tail(raw string, max int) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= max {
		return raw
	}
	return raw[len(raw)-max:]
}
