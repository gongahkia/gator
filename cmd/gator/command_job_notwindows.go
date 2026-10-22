//go:build darwin || linux

package main

import (
	"errors"
	"io"
)

const windowsRunPath = "Windows per-user start-at-login entry"

func loadLoginProfile() error {
	return errors.New("Windows start-at-login profiles are unavailable on this platform")
}
func reviewWindowsLogin(string) (bool, bool, string, error) {
	return false, false, "", loadLoginProfile()
}
func jobWindows([]string, io.Writer) error { return loadLoginProfile() }
