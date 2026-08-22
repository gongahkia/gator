package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/mcp"
)

const mcpUsage = `usage:
  gator mcp status
  gator mcp trust
  gator mcp untrust
  gator mcp login [--client-id CLIENT_ID] [--redirect-url LOOPBACK_URL] SERVER
  gator mcp logout SERVER`

func mcpCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
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
	if arguments[0] == "login" {
		return mcpLogin(arguments[1:], out, repository, settings, digest, hashErr)
	}
	if arguments[0] == "logout" {
		return mcpLogout(arguments[1:], out, repository)
	}
	if len(arguments) != 1 {
		return errors.New(mcpUsage)
	}
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
		if _, err := fmt.Fprintf(out, "Project MCP servers: %s\n  hash: %s\n  state: %s\n", repository, digest, state); err != nil {
			return err
		}
		credentials, err := gatorCredentials()
		if err != nil {
			return err
		}
		statuses, err := mcp.HTTPAuthorizationStatuses(repository, credentials, time.Now())
		if err != nil {
			return err
		}
		for _, status := range statuses {
			authentication := "not authenticated"
			if status.Authenticated {
				authentication = "authenticated"
			} else if status.Expired {
				authentication = "expired; run gator mcp login " + status.Server
			}
			if _, err := fmt.Fprintf(out, "  OAuth %s: %s\n", status.Server, authentication); err != nil {
				return err
			}
		}
		return nil
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

func mcpLogin(arguments []string, out io.Writer, repository string, settings config.Settings, digest string, hashErr error) error {
	flags := flag.NewFlagSet("gator mcp login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	clientID := flags.String("client-id", "", "pre-registered public OAuth client ID")
	redirectURL := flags.String("redirect-url", "", "exact loopback redirect URI registered to the public client")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 1 {
		return errors.New(mcpUsage)
	}
	if hashErr != nil {
		return hashErr
	}
	if digest == "" {
		return errors.New("no .gator/mcp.json exists to authenticate")
	}
	if mcpTrustFor(settings.MCPTrusts, repository) != digest {
		return errors.New("project MCP configuration is not trusted; review it and run 'gator mcp trust' before authenticating a remote server")
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	login, err := mcp.BeginOAuthLogin(context.Background(), repository, flags.Arg(0), digest, credentials, mcp.OAuthLoginOptions{ClientID: *clientID, RedirectURL: *redirectURL})
	if err != nil {
		return err
	}
	defer login.Cancel()
	if _, err := fmt.Fprintf(out, "Open this URL to authenticate Gator with MCP server %s:\n%s\n\nWaiting for the local callback...\n", flags.Arg(0), login.URL()); err != nil {
		return err
	}
	loginContext, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := login.Complete(loginContext); err != nil {
		return fmt.Errorf("complete MCP OAuth login: %w", err)
	}
	_, err = fmt.Fprintf(out, "Stored a Gator OAuth credential bound to MCP server %s.\n", flags.Arg(0))
	return err
}

func mcpLogout(arguments []string, out io.Writer, repository string) error {
	if len(arguments) != 1 || strings.TrimSpace(arguments[0]) == "" {
		return errors.New(mcpUsage)
	}
	key, err := mcp.OAuthCredentialKeyForServer(repository, arguments[0])
	if err != nil {
		return err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	if err := credentials.Delete(key); err != nil {
		return fmt.Errorf("remove MCP OAuth credential: %w", err)
	}
	_, err = fmt.Fprintf(out, "Removed the Gator OAuth credential for MCP server %s.\n", arguments[0])
	return err
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
