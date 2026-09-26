package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"

	internalacp "github.com/gongahkia/gator/internal/acp"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workrun"
)

// acpMode starts Gator as a local stdio ACP agent. Verification is process
// configuration rather than a client field: ACP clients must not be able to
// weaken completion policy per prompt without the developer configuring it.
func acpMode(arguments []string, input io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("acp", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var verification verificationFlags
	flags.Var(&verification, "verify", "required verification command as a whitespace-separated argv (repeatable)")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("usage: gator agent acp [--verify 'argv ...']")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	repository, err := gitRepositoryRoot(workingDirectory)
	if err != nil {
		return errors.New("ACP mode must start inside a Git checkout")
	}
	if len(verification) == 0 {
		verification = parseSuggestedVerification(suggestedVerificationCommands(repository))
	}
	defaults, err := configuredDefaults()
	if err != nil {
		return err
	}
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	server, err := internalacp.New(internalacp.Config{
		Input: input, Output: out, RepositoryPath: repository,
		DefaultProvider: defaults.Provider, DefaultModel: defaults.Model,
		DefaultVerification: verification, AgentVersion: version, ResolveProvider: resolveConfiguredProvider,
		NewWorkService: func(provider, modelName string, request *workrun.Request) (workrun.Service, error) {
			return configuredWorkService(provider, modelName, stateDir, request)
		},
	})
	if err != nil {
		return err
	}
	return server.Serve(context.Background())
}
