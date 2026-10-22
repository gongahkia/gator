package main

import (
	"path/filepath"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/state"
)

func preparePlatformProfile() error {
	root, err := state.ResolveDir("")
	if err != nil {
		return err
	}
	if err := privatefs.MkdirAll(filepath.Join(root, "gator")); err != nil {
		return err
	}
	configuration, err := config.ResolveDir("")
	if err != nil {
		return err
	}
	return privatefs.MkdirAll(filepath.Join(configuration, "gator"))
}
