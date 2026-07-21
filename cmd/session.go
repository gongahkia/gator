package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gongahkia/paw/internal/session"
	"github.com/spf13/cobra"
)

var sessionJSON bool

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Inspect persisted paw sessions",
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions in the current workspace",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		manifests, err := session.List(cwd)
		if err != nil {
			return err
		}
		if sessionJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(manifests)
		}
		for _, manifest := range manifests {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", manifest.ID, manifest.Status, manifest.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")); err != nil {
				return err
			}
		}
		return nil
	},
}

var sessionShowCmd = &cobra.Command{
	Use:   "show <session-id>",
	Short: "Show a persisted session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, err := session.Open(cwd, args[0])
		if err != nil {
			return err
		}
		manifest, err := store.LoadManifest()
		if err != nil {
			return err
		}
		events, err := store.Events()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if sessionJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				Manifest session.Manifest `json:"manifest"`
				Events   []session.Event  `json:"events"`
			}{Manifest: manifest, Events: events})
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s workspace=%s events=%d\n", manifest.ID, manifest.Status, manifest.Workspace.Root, len(events)); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionShowCmd)
	sessionListCmd.Flags().BoolVar(&sessionJSON, "json", false, "write JSON")
	sessionShowCmd.Flags().BoolVar(&sessionJSON, "json", false, "write JSON")
}
