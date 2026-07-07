package cmd

import (
	"github.com/gongahkia/paw/internal/config"
	editstage "github.com/gongahkia/paw/internal/edit"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Apply the planned edit",
	Example: `  paw gather --instruction "fix the failing test" | paw compress | paw plan | paw edit > .paw/edit.json
  paw gather --instruction "fix the failing test" | paw compress | paw plan | paw edit | paw verify`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		env, err := readEnvelope(cmd)
		if err != nil {
			return err
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		brain, err := llm.NewBrainClient(llmConfig(cfg))
		if err != nil {
			return err
		}
		out, err := editstage.New(brain).Run(cmd.Context(), env)
		if err != nil {
			return err
		}
		return writeEnvelope(cmd, out)
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
