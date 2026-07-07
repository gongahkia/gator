package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/spf13/cobra"
)

type endpointHealth interface {
	Check(context.Context, llm.EndpointConfig) llm.HealthReport
}

var doctorHealthChecker endpointHealth = llm.EndpointHealthChecker{}

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	Short:   "Check local paw setup",
	Example: "  paw doctor models",
}

var doctorModelsCmd = &cobra.Command{
	Use:     "models",
	Short:   "Check configured model transports",
	Example: "  paw doctor models",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		checker := doctorHealthChecker
		if concrete, ok := checker.(llm.EndpointHealthChecker); ok {
			concrete.AutoPullOllama = cfg.OllamaAutoPull
			concrete.SchemaSmokeOllama = true
			checker = concrete
		}
		reports := []namedHealthReport{
			{Name: "brain", Report: checker.Check(cmd.Context(), llmConfig(cfg).Brain)},
			{Name: "drone", Report: checker.Check(cmd.Context(), llmConfig(cfg).Drone)},
		}
		if err := writeHealthReports(cmd.OutOrStdout(), reports); err != nil {
			return err
		}
		if hasHealthFailures(reports) {
			return fmt.Errorf("model doctor found failures")
		}
		return nil
	},
}

type namedHealthReport struct {
	Name   string
	Report llm.HealthReport
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.AddCommand(doctorModelsCmd)
}

func writeHealthReports(w io.Writer, reports []namedHealthReport) error {
	for _, named := range reports {
		report := named.Report
		if _, err := fmt.Fprintf(w, "%s: %s", named.Name, report.Transport); err != nil {
			return err
		}
		if report.Model != "" {
			if _, err := fmt.Fprintf(w, " model=%s", report.Model); err != nil {
				return err
			}
		}
		if report.BaseURL != "" {
			if _, err := fmt.Fprintf(w, " base_url=%s", report.BaseURL); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		for _, check := range report.Checks {
			if _, err := fmt.Fprintf(w, "  %s %s", check.Status, check.Name); err != nil {
				return err
			}
			if check.Detail != "" {
				if _, err := fmt.Fprintf(w, ": %s", check.Detail); err != nil {
					return err
				}
			}
			if check.Action != "" {
				if _, err := fmt.Fprintf(w, " | action: %s", check.Action); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasHealthFailures(reports []namedHealthReport) bool {
	for _, named := range reports {
		for _, check := range named.Report.Checks {
			if check.Status == llm.HealthFail {
				return true
			}
		}
	}
	return false
}
