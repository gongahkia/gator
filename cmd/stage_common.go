package cmd

import (
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/spf13/cobra"
)

func readEnvelope(cmd *cobra.Command) (*envelope.Envelope, error) {
	return envelope.Unmarshal(cmd.InOrStdin())
}

func writeEnvelope(cmd *cobra.Command, env *envelope.Envelope) error {
	return env.Marshal(cmd.OutOrStdout())
}
