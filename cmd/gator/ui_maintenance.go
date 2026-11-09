package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/gongahkia/gator/internal/state"
)

// The host keeps this pipe open only while reading references and pruning.
// The existing profile guard covers the entire operation, not just its scan.
func uiMaintenanceLease(in io.Reader, out io.Writer) error {
	if os.Getenv("GATOR_UI_BACKEND") != "1" {
		return errors.New("maintenance lease is an internal Gator transport")
	}
	directory, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	release, err := state.Guard(directory, true)
	if errors.Is(err, state.ErrBusy) {
		return json.NewEncoder(out).Encode(map[string]bool{"ready": false})
	}
	if err != nil {
		return err
	}
	defer release()
	if err := json.NewEncoder(out).Encode(map[string]bool{"ready": true}); err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, io.LimitReader(in, 1))
	return err
}
