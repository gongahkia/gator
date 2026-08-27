package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/lsp"
)

const lspUsage = `usage:
  gator lsp status
  gator lsp trust
  gator lsp untrust`

func lspCommand(arguments []string, out io.Writer) error {
	if len(arguments) != 1 {
		return errors.New(lspUsage)
	}
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	repository, err := gitRepositoryRoot(directory)
	if err != nil {
		return errors.New("project LSP servers can be managed only from inside a Git checkout")
	}
	repository, err = lsp.CanonicalRepository(repository)
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
	digest, hashErr := lsp.BundleHash(repository)
	switch arguments[0] {
	case "status":
		if hashErr != nil {
			return hashErr
		}
		if digest == "" {
			_, err := fmt.Fprintln(out, "No project LSP servers are configured.")
			return err
		}
		state := "disabled (hash is not trusted)"
		if lspTrustFor(settings.LSPTrusts, repository) == digest {
			state = "active"
		}
		if _, err := fmt.Fprintf(out, "Project LSP servers: %s\n  hash: %s\n  state: %s\n  session cache: none\n", repository, digest, state); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "  Status never starts a language server. The first approved lookup starts one for a trusted hash.")
		return err
	case "trust":
		if hashErr != nil {
			return hashErr
		}
		if digest == "" {
			return errors.New("no .gator/lsp.json exists to trust")
		}
		settings.LSPTrusts = setLSPTrust(settings.LSPTrusts, lsp.Trust{Repository: repository, Hash: digest})
		if err := store.Save(settings); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Trusted project LSP bundle %s. Each LSP lookup still requests approval; the first approved lookup starts its server.\n", digest)
		return err
	case "untrust":
		settings.LSPTrusts = removeLSPTrust(settings.LSPTrusts, repository)
		if err := store.Save(settings); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Stopped trusting project LSP servers in %s.\n", repository)
		return err
	default:
		return fmt.Errorf("unknown LSP command %q\n%s", arguments[0], lspUsage)
	}
}

func lspTrustFor(trusts []lsp.Trust, repository string) string {
	for _, trust := range trusts {
		if trust.Repository == repository {
			return trust.Hash
		}
	}
	return ""
}

func setLSPTrust(trusts []lsp.Trust, trust lsp.Trust) []lsp.Trust {
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

func removeLSPTrust(trusts []lsp.Trust, repository string) []lsp.Trust {
	result := trusts[:0]
	for _, trust := range trusts {
		if trust.Repository != repository {
			result = append(result, trust)
		}
	}
	return result
}
