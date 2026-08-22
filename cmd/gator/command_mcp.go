package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/mcp"
)

const mcpUsage = `usage:
  gator mcp status
  gator mcp trust
  gator mcp untrust`

func mcpCommand(arguments []string, out io.Writer) error {
	if len(arguments) != 1 {
		return errors.New(mcpUsage)
	}
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	repository, err := gitRepositoryRoot(directory)
	if err != nil {
		return errors.New("project MCP servers can be managed only from inside a Git checkout")
	}
	repository, err = mcp.CanonicalRepository(repository)
	if err != nil {
		return err
	}
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	settings, err := store.Load()
	if err != nil {
		return err
	}
	digest, hashErr := mcp.BundleHash(repository)
	switch arguments[0] {
	case "status":
		if hashErr != nil {
			return hashErr
		}
		if digest == "" {
			_, err := fmt.Fprintln(out, "No project MCP servers are configured.")
			return err
		}
		state := "disabled (hash is not trusted)"
		if mcpTrustFor(settings.MCPTrusts, repository) == digest {
			state = "active"
		}
		_, err := fmt.Fprintf(out, "Project MCP servers: %s\n  hash: %s\n  state: %s\n", repository, digest, state)
		return err
	case "trust":
		if hashErr != nil {
			return hashErr
		}
		if digest == "" {
			return errors.New("no .gator/mcp.json exists to trust")
		}
		settings.MCPTrusts = setMCPTrust(settings.MCPTrusts, mcp.Trust{Repository: repository, Hash: digest})
		if err := store.Save(settings); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Trusted project MCP bundle %s. Every MCP tool still requests approval before use.\n", digest)
		return err
	case "untrust":
		settings.MCPTrusts = removeMCPTrust(settings.MCPTrusts, repository)
		if err := store.Save(settings); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Stopped trusting project MCP servers in %s.\n", repository)
		return err
	default:
		return fmt.Errorf("unknown MCP command %q\n%s", arguments[0], mcpUsage)
	}
}

func mcpTrustFor(trusts []mcp.Trust, repository string) string {
	for _, trust := range trusts {
		if trust.Repository == repository {
			return trust.Hash
		}
	}
	return ""
}

func setMCPTrust(trusts []mcp.Trust, trust mcp.Trust) []mcp.Trust {
	for index := range trusts {
		if trusts[index].Repository == trust.Repository {
			trusts[index] = trust
			return trusts
		}
	}
	trusts = append(trusts, trust)
	sort.Slice(trusts, func(first, second int) bool { return trusts[first].Repository < trusts[second].Repository })
	return trusts
}

func removeMCPTrust(trusts []mcp.Trust, repository string) []mcp.Trust {
	result := trusts[:0]
	for _, trust := range trusts {
		if trust.Repository != repository {
			result = append(result, trust)
		}
	}
	return result
}
