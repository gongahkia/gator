package main

import (
	"errors"
	"os"
)

// Service edits from Recovery carry the reviewed digest into the subprocess.
// Recheck there too: the configuration may change while the old supervisor stops.
func checkJobServiceChange(path, stateDir, platform, expected string) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("background service configuration is not an ordinary file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	installed, owns, digest, err := recoveryServiceReview(path, stateDir, platform)
	if err != nil {
		return err
	}
	if expected != "" && expected != digest {
		return errors.New("background service changed during review; refresh Recovery")
	}
	if installed && !owns {
		return errors.New("background service belongs to another or unrecognized profile")
	}
	return nil
}

// Keep the explicit command-line install/remove forms; the UI additionally
// supplies --expected to bind its reviewed operation to the configuration.
func jobServiceChangeArguments(arguments []string) (string, error) {
	if len(arguments) == 0 {
		return "", errors.New("missing service action")
	}
	remaining := arguments[1:]
	if arguments[0] == "remove" {
		if len(remaining) == 0 || remaining[0] != "--yes" {
			return "", errors.New("service removal requires --yes")
		}
		remaining = remaining[1:]
	} else if arguments[0] != "install" {
		return "", errors.New("unknown service change")
	}
	if len(remaining) == 0 {
		return "", nil
	}
	if len(remaining) != 2 || remaining[0] != "--expected" || remaining[1] == "" {
		return "", errors.New("expected optional --expected SERVICE_DIGEST")
	}
	return remaining[1], nil
}
