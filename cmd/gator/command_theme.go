package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/config"
)

const themeUsage = "usage: gator theme list | gator theme set gator|contrast|mono"

func themeCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 1 && arguments[0] == "list" {
		_, err := fmt.Fprintln(out, "gator\ncontrast\nmono")
		return err
	}
	if len(arguments) != 2 || arguments[0] != "set" {
		return errors.New(themeUsage)
	}
	if err := saveTheme(arguments[1]); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "Theme set to %q.\n", strings.ToLower(strings.TrimSpace(arguments[1])))
	return err
}

func saveTheme(name string) error {
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	settings, err := store.Load()
	if err != nil {
		return err
	}
	settings.Theme = strings.ToLower(strings.TrimSpace(name))
	return store.Save(settings)
}
