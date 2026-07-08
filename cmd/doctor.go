package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/gongahkia/paw/internal/config"
	doctorpkg "github.com/gongahkia/paw/internal/doctor"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/spf13/cobra"
)

type endpointHealth interface {
	Check(context.Context, llm.EndpointConfig) llm.HealthReport
}

var doctorHealthChecker endpointHealth = llm.EndpointHealthChecker{}

var (
	doctorJSON           bool
	doctorLint           bool
	doctorDeep           bool
	doctorFix            bool
	doctorDryRun         bool
	doctorYes            bool
	doctorNonInteractive bool
	doctorSeverityMin    string
	doctorOnly           []string
	doctorSkip           []string
	doctorHistoryJSON    bool
	doctorHistoryLimit   int
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check local paw setup",
	Example: `  paw doctor
  paw doctor --fix --dry-run
  paw doctor --fix --yes
  paw doctor --lint --json
  paw doctor history
  paw doctor explain config.bootstrap
  paw doctor models`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if doctorLint && doctorFix {
			return usageErrorf("use --lint without --fix")
		}
		severityMin, err := doctorpkg.ParseSeverity(doctorSeverityMin)
		if err != nil {
			return usageError(err)
		}
		report := doctorpkg.Run(cmd.Context(), doctorpkg.Options{
			ConfigPath:     configPath,
			Version:        version,
			Deep:           doctorDeep,
			Fix:            doctorFix,
			DryRun:         doctorDryRun,
			Yes:            doctorYes,
			NonInteractive: doctorNonInteractive,
			SeverityMin:    severityMin,
			Only:           doctorOnly,
			Skip:           doctorSkip,
			HealthChecker:  doctorHealthChecker,
			Stdin:          cmd.InOrStdin(),
			PromptWriter:   cmd.ErrOrStderr(),
		})
		if doctorJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			if err := enc.Encode(report); err != nil {
				return err
			}
		} else if err := writeDoctorReport(cmd.OutOrStdout(), report); err != nil {
			return err
		}
		if doctorLint {
			threshold := severityMin
			if threshold == doctorpkg.SeverityInfo {
				threshold = doctorpkg.SeverityWarning
			}
			if report.HasIssues(threshold) {
				return fmt.Errorf("doctor lint found issues")
			}
			return nil
		}
		if report.HasIssues(doctorpkg.SeverityError) {
			return fmt.Errorf("doctor found failures")
		}
		return nil
	},
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
		return runDoctorModels(cmd.Context(), cmd.OutOrStdout(), cfg)
	},
}

var doctorHistoryCmd = &cobra.Command{
	Use:     "history",
	Short:   "Show doctor repair operation history",
	Example: "  paw doctor history\n  paw doctor history --json",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		ops, err := doctorpkg.ReadOperations(cwd, doctorHistoryLimit)
		if err != nil {
			return err
		}
		if doctorHistoryJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(ops)
		}
		if len(ops) == 0 {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "no doctor repair operations")
			return err
		}
		for _, op := range ops {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s", op.Timestamp, op.Status, op.Action); err != nil {
				return err
			}
			if op.Path != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), " path=%s", op.Path); err != nil {
					return err
				}
			}
			if op.Error != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), " error=%q", op.Error); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return err
			}
		}
		return nil
	},
}

var doctorExplainCmd = &cobra.Command{
	Use:     "explain <finding-id>",
	Short:   "Explain a doctor finding",
	Args:    cobra.ExactArgs(1),
	Example: "  paw doctor explain config.bootstrap",
	RunE: func(cmd *cobra.Command, args []string) error {
		text, ok := doctorpkg.Explanation(args[0])
		if !ok {
			return usageErrorf("unknown doctor finding %q", args[0])
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", args[0], text)
		return err
	},
}

type namedHealthReport struct {
	Name   string
	Report llm.HealthReport
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.AddCommand(doctorModelsCmd)
	doctorCmd.AddCommand(doctorHistoryCmd)
	doctorCmd.AddCommand(doctorExplainCmd)
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "write machine-readable JSON report")
	doctorCmd.Flags().BoolVar(&doctorLint, "lint", false, "read-only CI preflight; fail on warnings or errors")
	doctorCmd.Flags().BoolVar(&doctorDeep, "deep", false, "run slower probes, including verify dry-run and schema smoke checks")
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "apply supported repairs")
	doctorCmd.Flags().BoolVar(&doctorDryRun, "dry-run", false, "preview repairs without changing files")
	doctorCmd.Flags().BoolVar(&doctorYes, "yes", false, "apply repairs without prompting")
	doctorCmd.Flags().BoolVar(&doctorNonInteractive, "non-interactive", false, "disable prompts and risky repair actions")
	doctorCmd.Flags().StringVar(&doctorSeverityMin, "severity-min", "info", "minimum severity to report: info, warning, error")
	doctorCmd.Flags().StringArrayVar(&doctorOnly, "only", nil, "only run matching check id or section; repeatable")
	doctorCmd.Flags().StringArrayVar(&doctorSkip, "skip", nil, "skip matching check id or section; repeatable")
	doctorHistoryCmd.Flags().BoolVar(&doctorHistoryJSON, "json", false, "write machine-readable JSON history")
	doctorHistoryCmd.Flags().IntVar(&doctorHistoryLimit, "limit", 20, "maximum operations to show")
}

func runDoctorModels(ctx context.Context, w io.Writer, cfg config.Config) error {
	checker := doctorHealthChecker
	if concrete, ok := checker.(llm.EndpointHealthChecker); ok {
		concrete.AutoPullOllama = cfg.OllamaAutoPull
		concrete.SchemaSmokeOllama = true
		checker = concrete
	}
	reports := []namedHealthReport{
		{Name: "brain", Report: checker.Check(ctx, llmConfig(cfg).Brain)},
		{Name: "drone", Report: checker.Check(ctx, llmConfig(cfg).Drone)},
	}
	if err := writeHealthReports(w, reports); err != nil {
		return err
	}
	if hasHealthFailures(reports) {
		return fmt.Errorf("model doctor found failures")
	}
	return nil
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

func writeDoctorReport(w io.Writer, report doctorpkg.Report) error {
	if _, err := fmt.Fprintf(w, "paw doctor: %s\n", report.CWD); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "summary: ok=%d warn=%d fail=%d fixed=%d plan=%d skip=%d\n", report.Summary.OK, report.Summary.Warning, report.Summary.Error, report.Summary.Fixed, report.Summary.Planned, report.Summary.Skipped); err != nil {
		return err
	}
	section := ""
	for _, finding := range report.Findings {
		if finding.Section != section {
			section = finding.Section
			if _, err := fmt.Fprintf(w, "\n[%s]\n", section); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "  %s %s: %s", finding.Status, finding.ID, finding.Message); err != nil {
			return err
		}
		if finding.Detail != "" {
			if _, err := fmt.Fprintf(w, " (%s)", finding.Detail); err != nil {
				return err
			}
		}
		if finding.Path != "" {
			if _, err := fmt.Fprintf(w, " path=%s", finding.Path); err != nil {
				return err
			}
		}
		if finding.Command != "" {
			if _, err := fmt.Fprintf(w, " run=%q", finding.Command); err != nil {
				return err
			}
		}
		if finding.Fix != "" {
			if _, err := fmt.Fprintf(w, " fix=%q", finding.Fix); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}
