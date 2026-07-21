package cmd

import (
	"github.com/gongahkia/paw/internal/config"
	planstage "github.com/gongahkia/paw/internal/plan"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Plan the next agent step",
	Example: `  paw gather --instruction "fix the failing test" | paw compress | paw plan > .paw/plan.json
  paw gather --instruction "fix the failing test" | paw compress | paw plan | paw edit`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		env, err := readEnvelope(cmd)
		if err != nil {
			return err
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		brain, err := newBrainClient(cfg)
		if err != nil {
			return err
		}
		out, err := planstage.New(brain).Run(cmd.Context(), env)
		if err != nil {
			return err
		}
		return writeEnvelope(cmd, out)
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
