package cmd

import (
	"github.com/gongahkia/paw/internal/compress"
	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/spf13/cobra"
)

var (
	compressDisableCompress bool
	compressDroneModel      string
)

var compressCmd = &cobra.Command{
	Use:   "compress",
	Short: "Compress raw context",
	Example: `  paw gather --instruction "fix the failing test" | paw compress > .paw/compressed.json
  paw gather --instruction "fix the failing test" | paw compress --disable-compress | paw plan`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		env, err := readEnvelope(cmd)
		if err != nil {
			return err
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		if compressDroneModel != "" {
			cfg.Drone.Model = compressDroneModel
		}
		var drone llm.Client
		if !compressDisableCompress {
			drone, err = llm.NewDroneClient(llmConfig(cfg))
			if err != nil {
				return err
			}
		}
		stage := compress.New(drone)
		stage.DisableCompress = compressDisableCompress
		out, err := stage.Run(cmd.Context(), env)
		if err != nil {
			return err
		}
		return writeEnvelope(cmd, out)
	},
}

func init() {
	rootCmd.AddCommand(compressCmd)
	compressCmd.Flags().BoolVar(&compressDisableCompress, "disable-compress", false, "use deterministic compression fallback")
	compressCmd.Flags().StringVar(&compressDroneModel, "drone-model", "", "drone model override")
}
