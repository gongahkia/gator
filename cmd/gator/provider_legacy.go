package main

import gatorrun "github.com/gongahkia/gator/internal/run"

// newExecutor remains only for legacy machine-facing transports and the
// retained legacy evaluator. Normal Work, including Work Code, uses
// newCodeExecutor and does not enter this compatibility path.
func newExecutor(providerName, modelName, baseURL string) (gatorrun.Executor, error) {
	configuration, err := newNativeExecutorConfiguration(providerName, modelName, baseURL)
	if err != nil {
		return gatorrun.Executor{}, err
	}
	return configuration.legacyExecutor(), nil
}

func (configuration nativeExecutorConfiguration) legacyExecutor() gatorrun.Executor {
	return gatorrun.Executor{
		Model: configuration.Model, Extensions: configuration.Extensions,
		HookTrusts: configuration.HookTrusts, LSPTrusts: configuration.LSPTrusts, MCPTrusts: configuration.MCPTrusts,
		MCPCredentials: configuration.MCPCredentials, HTTP: configuration.HTTP, Sandbox: configuration.Sandbox,
	}
}
