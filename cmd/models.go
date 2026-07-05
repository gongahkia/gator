package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/spf13/cobra"
)

type endpointModelLister interface {
	ListEndpointModels(context.Context, llm.EndpointConfig, llm.ModelListOptions) ([]llm.ModelInfo, error)
}

var (
	modelsLister       endpointModelLister = llm.EndpointHealthChecker{}
	modelsListQuery    string
	modelsListProvider string
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Inspect configured models",
}

var modelsListCmd = &cobra.Command{
	Use:   "list [brain|drone]",
	Short: "List models for configured transports",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		endpoints, err := selectedModelEndpoints(cfg, args)
		if err != nil {
			return err
		}
		opts := llm.ModelListOptions{Query: modelsListQuery, Provider: modelsListProvider}
		var failed bool
		for _, endpoint := range endpoints {
			models, err := modelsLister.ListEndpointModels(cmd.Context(), endpoint.Endpoint, opts)
			if err != nil {
				failed = true
				fmt.Fprintf(cmd.OutOrStdout(), "%s: error: %v\n", endpoint.Name, err)
				continue
			}
			writeModelList(cmd.OutOrStdout(), endpoint.Name, endpoint.Endpoint.Transport, models)
		}
		if failed {
			return fmt.Errorf("model listing failed")
		}
		return nil
	},
}

type namedEndpoint struct {
	Name     string
	Endpoint llm.EndpointConfig
}

func init() {
	rootCmd.AddCommand(modelsCmd)
	modelsCmd.AddCommand(modelsListCmd)
	modelsListCmd.Flags().StringVar(&modelsListQuery, "query", "", "partial model name for transports that require search")
	modelsListCmd.Flags().StringVar(&modelsListProvider, "provider", "", "provider filter for transports that support it")
}

func selectedModelEndpoints(cfg config.Config, args []string) ([]namedEndpoint, error) {
	llmCfg := llmConfig(cfg)
	all := []namedEndpoint{
		{Name: "brain", Endpoint: llmCfg.Brain},
		{Name: "drone", Endpoint: llmCfg.Drone},
	}
	if len(args) == 0 {
		return all, nil
	}
	switch args[0] {
	case "brain":
		return all[:1], nil
	case "drone":
		return all[1:], nil
	default:
		return nil, fmt.Errorf("unknown model endpoint %q", args[0])
	}
}

func writeModelList(w io.Writer, name, transport string, models []llm.ModelInfo) {
	fmt.Fprintf(w, "%s: %s\n", name, transport)
	if len(models) == 0 {
		fmt.Fprintln(w, "  no models returned")
		return
	}
	for _, model := range models {
		fmt.Fprintf(w, "  %s\n", model.ID)
	}
}
