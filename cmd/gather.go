package cmd

import (
	"os"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
	gatherstage "github.com/gongahkia/paw/internal/gather"
	"github.com/spf13/cobra"
)

var gatherInstruction string

var gatherCmd = &cobra.Command{
	Use:   "gather",
	Short: "Gather raw task context",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if gatherInstruction == "" {
			return usageErrorf("missing --instruction")
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		env := envelope.NewEnvelope(taskID(gatherInstruction, cwd), gatherInstruction, cwd)
		out, err := gatherstage.New(cfg.Gather).Run(cmd.Context(), env)
		if err != nil {
			return err
		}
		return writeEnvelope(cmd, out)
	},
}

func init() {
	rootCmd.AddCommand(gatherCmd)
	gatherCmd.Flags().StringVar(&gatherInstruction, "instruction", "", "task instruction")
}
