package main

import "github.com/gongahkia/gator/internal/config"

func saveWorkStatusLine(store config.Store) func(*[]string) error {
	return func(items *[]string) error {
		settings, err := store.Load()
		if err != nil {
			return err
		}
		if items == nil {
			settings.TUI.StatusLine = nil
		} else {
			copyOfItems := make([]string, len(*items))
			copy(copyOfItems, *items)
			settings.TUI.StatusLine = &copyOfItems
		}
		return store.Save(settings)
	}
}
