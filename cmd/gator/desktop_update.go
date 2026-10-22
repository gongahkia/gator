package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gongahkia/gator/internal/state"
)

// The host stops its idle foreground session before asking for this lease.
// Existing writers are never cancelled. A supervisor must be stopped through
// Recovery first, even if it happens to be between checks at this instant.
func holdDesktopUpdateGuard(in io.Reader, out io.Writer) error {
	root, err := state.ResolveDir("")
	if err != nil {
		return err
	}
	release, err := state.Guard(root, true)
	if err != nil {
		return errors.New("other work is using this profile; finish it before updating Gator")
	}
	defer release()
	path := filepath.Join(root, "gator", "jobs", "supervisor.lock")
	payload, err := os.ReadFile(path)
	if err == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(payload)))
		if parseErr != nil || pid < 1 {
			return errors.New("background-check state needs attention in Recovery before updating")
		}
		if processAlive(pid) {
			return errors.New("stop background checks in Settings > Recovery before updating; saved checks will be retained")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot inspect background checks before updating: %w", err)
	}
	if err := json.NewEncoder(out).Encode(struct {
		Ready bool `json:"ready"`
	}{Ready: true}); err != nil {
		return err
	}
	// EOF releases the lease. No data migrations, worker restarts, scheduling
	// changes or profile publication are performed by this private helper.
	_, err = io.Copy(io.Discard, in)
	return err
}
