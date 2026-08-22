package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/hooks"
)

const hookUsage = `usage:
  gator hook status
  gator hook trust
  gator hook untrust`

func hookCommand(arguments []string, out io.Writer) error {
	if len(arguments) != 1 {
		return errors.New(hookUsage)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	repository, err := gitRepositoryRoot(workingDirectory)
	if err != nil {
		return errors.New("project hooks can be managed only from inside a Git checkout")
	}
	repository, err = hooks.CanonicalRepository(repository)
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
	digest, hashErr := hooks.BundleHash(repository)
	trusted := hookTrustFor(settings.HookTrusts, repository)
	switch arguments[0] {
	case "status":
		if hashErr != nil {
			return hashErr
		}
		if digest == "" {
			_, err := fmt.Fprintln(out, "No project hooks are configured.")
			return err
		}
		state := "disabled (hash is not trusted)"
		if trusted == digest {
			state = "active"
		}
		_, err := fmt.Fprintf(out, "Project hooks: %s\n  hash: %s\n  state: %s\n", repository, digest, state)
		return err
	case "trust":
		if hashErr != nil {
			return hashErr
		}
		if digest == "" {
			return errors.New("no .gator/hooks.json exists to trust")
		}
		settings.HookTrusts = setHookTrust(settings.HookTrusts, hooks.Trust{Repository: repository, Hash: digest})
		if err := store.Save(settings); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Trusted project hook bundle %s. Any manifest or declared executable change disables hooks until you trust its new hash.\n", digest)
		return err
	case "untrust":
		settings.HookTrusts = removeHookTrust(settings.HookTrusts, repository)
		if err := store.Save(settings); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Stopped trusting project hooks in %s.\n", repository)
		return err
	default:
		return fmt.Errorf("unknown hook command %q\n%s", arguments[0], hookUsage)
	}
}

func hookTrustFor(trusts []hooks.Trust, repository string) string {
	for _, trust := range trusts {
		if trust.Repository == repository {
			return trust.Hash
		}
	}
	return ""
}

func setHookTrust(trusts []hooks.Trust, trust hooks.Trust) []hooks.Trust {
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

func removeHookTrust(trusts []hooks.Trust, repository string) []hooks.Trust {
	result := trusts[:0]
	for _, trust := range trusts {
		if trust.Repository != repository {
			result = append(result, trust)
		}
	}
	return result
}
