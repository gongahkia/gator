package cmd

import (
	"github.com/gongahkia/paw/internal/config"
	mcppkg "github.com/gongahkia/paw/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run MCP integrations",
}

var mcpServeDisableCompress bool

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run a stdio MCP server for context tools",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		server := mcppkg.NewServer(mcppkg.StageRunner{Config: cfg, DisableCompress: mcpServeDisableCompress})
		return server.Serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpServeCmd)
	mcpServeCmd.Flags().BoolVar(&mcpServeDisableCompress, "disable-compress", false, "use deterministic compression fallback")
}
