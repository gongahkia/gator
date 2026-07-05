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
	Use:   "doctor",
	Short: "Check local paw setup",
}

var doctorModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Check configured model transports",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		reports := []namedHealthReport{
			{Name: "brain", Report: doctorHealthChecker.Check(cmd.Context(), llmConfig(cfg).Brain)},
			{Name: "drone", Report: doctorHealthChecker.Check(cmd.Context(), llmConfig(cfg).Drone)},
		}
		writeHealthReports(cmd.OutOrStdout(), reports)
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

func writeHealthReports(w io.Writer, reports []namedHealthReport) {
	for _, named := range reports {
		report := named.Report
		fmt.Fprintf(w, "%s: %s", named.Name, report.Transport)
		if report.Model != "" {
			fmt.Fprintf(w, " model=%s", report.Model)
		}
		if report.BaseURL != "" {
			fmt.Fprintf(w, " base_url=%s", report.BaseURL)
		}
		fmt.Fprintln(w)
		for _, check := range report.Checks {
			fmt.Fprintf(w, "  %s %s", check.Status, check.Name)
			if check.Detail != "" {
				fmt.Fprintf(w, ": %s", check.Detail)
			}
			if check.Action != "" {
				fmt.Fprintf(w, " | action: %s", check.Action)
			}
			fmt.Fprintln(w)
		}
	}
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
