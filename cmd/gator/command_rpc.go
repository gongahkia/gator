package main

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/gongahkia/gator/internal/journal"
	internalrpc "github.com/gongahkia/gator/internal/rpc"
)

func rpcMode(arguments []string, input io.Reader, out io.Writer) error {
	if len(arguments) != 0 {
		return errors.New("usage: gator rpc")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	repository, err := gitRepositoryRoot(workingDirectory)
	if err != nil {
		return errors.New("RPC mode must start inside a Git checkout")
	}
	defaults, err := configuredDefaults()
	if err != nil {
		return err
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	server, err := internalrpc.New(internalrpc.Config{
		Input: input, Output: out, RepositoryPath: repository, StateDir: stateDir,
		DefaultProvider: defaults.Provider, DefaultModel: defaults.Model, ResolveProvider: resolveConfiguredProvider, NewExecutor: newExecutor,
	})
	if err != nil {
		return err
	}
	return server.Serve(context.Background())
}
