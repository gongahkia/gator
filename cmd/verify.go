package cmd

import (
	"github.com/gongahkia/paw/internal/config"
	verifystage "github.com/gongahkia/paw/internal/verify"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Run verification command",
	Example: `  paw gather --instruction "fix the failing test" | paw compress | paw plan | paw edit | paw verify
  paw edit < .paw/plan.json | paw verify > .paw/verified.json`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		env, err := readEnvelope(cmd)
		if err != nil {
			return err
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		stage := verifystage.New(cfg.Verify.Command)
		stage.Timeout = cfg.Verify.Timeout
		out, err := stage.Run(cmd.Context(), env)
		if err != nil {
			return err
		}
		return writeEnvelope(cmd, out)
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}
