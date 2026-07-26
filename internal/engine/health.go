package engine

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/runtime"
)

func (s *Service) Health(ctx context.Context) domain.HealthReport {
	report := domain.HealthReport{CheckedAt: time.Now().UTC()}
	report.Checks = append(report.Checks, s.checkPostgres(ctx), s.checkQueue(ctx), s.checkRuntime(ctx))
	for _, provider := range s.config.Manifest.Providers {
		report.Checks = append(report.Checks, s.checkProvider(ctx, provider.ID, provider.BaseURL, provider.CredentialEnv, provider.Kind))
	}
	if imports, err := s.store.SkillImports(ctx); err != nil {
		report.Checks = append(report.Checks, failedCheck("marketplace", true, err))
	} else {
		report.Checks = append(report.Checks, marketplaceCheck(imports))
	}
	if accounts, err := s.store.ChannelAccounts(ctx); err != nil {
		report.Checks = append(report.Checks, failedCheck("channels", false, err))
	} else {
		report.Checks = append(report.Checks, s.checkChannels(accounts))
	}
	runs, err := s.store.ListRuns(ctx)
	if err != nil {
		report.Checks = append(report.Checks, failedCheck("deployments", false, err))
	} else {
		for _, run := range runs {
			if _, err := s.store.GetDeployment(ctx, run.ID); err != nil {
				continue
			}
			report.Checks = append(report.Checks, s.checkDeployment(ctx, run))
		}
	}
	report.State = domain.HealthHealthy
	for _, check := range report.Checks {
		if check.State == domain.HealthDown && check.Critical {
			report.State = domain.HealthDown
			break
		}
		if check.State != domain.HealthHealthy && report.State == domain.HealthHealthy {
			report.State = domain.HealthDegraded
		}
	}
	return report
}

func (s *Service) checkChannels(accounts []domain.ChannelAccount) domain.HealthCheck {
	started := time.Now()
	if len(accounts) == 0 {
		return okCheck("channels", false, "no configured channel accounts", started, nil)
	}
	missing := []string{}
	for _, account := range accounts {
		if !account.Enabled {
			continue
		}
		for name, ref := range account.SecretRefs {
			if strings.TrimSpace(os.Getenv(ref)) == "" {
				missing = append(missing, account.ID+":"+name)
			}
		}
	}
	if len(missing) > 0 {
		return domain.HealthCheck{ID: "channels", State: domain.HealthDown, Critical: true, LatencyMS: time.Since(started).Milliseconds(), Message: "channel secret references are unset", Diagnostics: map[string]any{"missing": missing, "accounts": len(accounts)}, CheckedAt: time.Now().UTC()}
	}
	return okCheck("channels", true, fmt.Sprintf("%d configured channel accounts", len(accounts)), started, map[string]any{"accounts": len(accounts)})
}

func (s *Service) checkPostgres(ctx context.Context) domain.HealthCheck {
	started := time.Now()
	err := s.store.Ping(ctx)
	if err != nil {
		return failedCheckWithLatency("postgres", true, err, started)
	}
	return okCheck("postgres", true, "reachable", started, nil)
}

func (s *Service) checkQueue(ctx context.Context) domain.HealthCheck {
	started := time.Now()
	depth, err := s.store.QueueDepth(ctx)
	if err != nil {
		return failedCheckWithLatency("queue", true, err, started)
	}
	state, message := domain.HealthHealthy, "no queued work"
	if depth > 0 {
		state, message = domain.HealthDegraded, fmt.Sprintf("%d queued or leased jobs", depth)
	}
	return domain.HealthCheck{ID: "queue", State: state, Critical: true, LatencyMS: time.Since(started).Milliseconds(), Message: message, Diagnostics: map[string]any{"depth": depth}, CheckedAt: time.Now().UTC()}
}

func (s *Service) checkRuntime(ctx context.Context) domain.HealthCheck {
	started := time.Now()
	if s.config.Manifest.DefaultTarget() == domain.DeploymentDocker {
		_, err := runtime.OSRunner{}.Run(ctx, s.config.DockerBin, "info", "--format", "{{.ServerVersion}}")
		if err != nil {
			return failedCheckWithLatency("runtime", true, err, started)
		}
		return okCheck("runtime", true, "Docker reachable", started, map[string]any{"target": "docker"})
	}
	kube, err := runtime.NewKubernetesRuntime(s.config.Manifest.Runtime.Kubernetes, s.config.ArtifactsDir)
	if err == nil {
		err = kube.Validate(ctx)
	}
	if err != nil {
		return failedCheckWithLatency("runtime", true, err, started)
	}
	return okCheck("runtime", true, "Kubernetes API reachable", started, map[string]any{"target": "kubernetes"})
}

func (s *Service) checkProvider(ctx context.Context, id, baseURL, credentialEnv, kind string) domain.HealthCheck {
	started := time.Now()
	if kind == "plugin" {
		return okCheck("provider:"+id, true, "locally configured", started, map[string]any{"kind": kind})
	}
	if credentialEnv != "" && strings.TrimSpace(os.Getenv(credentialEnv)) == "" {
		return domain.HealthCheck{ID: "provider:" + id, State: domain.HealthDown, Critical: true, LatencyMS: time.Since(started).Milliseconds(), Message: "credential env is unset", Diagnostics: map[string]any{"credential_env": credentialEnv}, CheckedAt: time.Now().UTC()}
	}
	if kind == "cli" {
		return okCheck("provider:"+id, true, "runner and credential configured", started, map[string]any{"kind": kind, "credential_env": credentialEnv})
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, baseURL, nil)
	if err != nil {
		return failedCheckWithLatency("provider:"+id, true, err, started)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return failedCheckWithLatency("provider:"+id, true, err, started)
	}
	response.Body.Close()
	state, message := domain.HealthHealthy, "endpoint reachable"
	if response.StatusCode >= 500 {
		state, message = domain.HealthDegraded, fmt.Sprintf("endpoint returned %d", response.StatusCode)
	}
	return domain.HealthCheck{ID: "provider:" + id, State: state, Critical: true, LatencyMS: time.Since(started).Milliseconds(), Message: message, Diagnostics: map[string]any{"kind": kind, "status_code": response.StatusCode}, CheckedAt: time.Now().UTC()}
}

func marketplaceCheck(imports []domain.SkillImport) domain.HealthCheck {
	state, message := domain.HealthHealthy, "no imported skills"
	if len(imports) > 0 {
		message = fmt.Sprintf("%d imported skill bundles", len(imports))
	}
	for _, imported := range imports {
		if imported.State != "active" {
			state, message = domain.HealthDegraded, "skill imports await activation"
			break
		}
	}
	return domain.HealthCheck{ID: "marketplace", State: state, Critical: false, Message: message, Diagnostics: map[string]any{"imports": len(imports)}, CheckedAt: time.Now().UTC()}
}

func (s *Service) checkDeployment(ctx context.Context, run domain.Run) domain.HealthCheck {
	started := time.Now()
	workspace, backend, err := s.backendForRun(ctx, run)
	if err == nil {
		_, err = backend.Status(ctx, run.ID, workspace.RunPath(run.ID))
	}
	if err != nil {
		logs := ""
		if backend != nil && workspace != nil {
			if tail, logErr := backend.Logs(ctx, run.ID, workspace.RunPath(run.ID), 80); logErr == nil {
				logs = s.redact(tail)
			}
		}
		return domain.HealthCheck{ID: "deployment:" + run.ID, State: domain.HealthDown, Critical: false, LatencyMS: time.Since(started).Milliseconds(), Message: "deployment probe failed", LogTail: logs, Diagnostics: map[string]any{"run_id": run.ID, "error": err.Error()}, CheckedAt: time.Now().UTC()}
	}
	return okCheck("deployment:"+run.ID, false, "runtime status reachable", started, map[string]any{"run_id": run.ID})
}

func (s *Service) redact(value string) string {
	for _, provider := range s.config.Manifest.Providers {
		if secret := strings.TrimSpace(os.Getenv(provider.CredentialEnv)); secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

func okCheck(id string, critical bool, message string, started time.Time, diagnostics map[string]any) domain.HealthCheck {
	return domain.HealthCheck{ID: id, State: domain.HealthHealthy, Critical: critical, LatencyMS: time.Since(started).Milliseconds(), Message: message, Diagnostics: diagnostics, CheckedAt: time.Now().UTC()}
}
func failedCheck(id string, critical bool, err error) domain.HealthCheck {
	return failedCheckWithLatency(id, critical, err, time.Now())
}
func failedCheckWithLatency(id string, critical bool, err error, started time.Time) domain.HealthCheck {
	return domain.HealthCheck{ID: id, State: domain.HealthDown, Critical: critical, LatencyMS: time.Since(started).Milliseconds(), Message: err.Error(), CheckedAt: time.Now().UTC()}
}
