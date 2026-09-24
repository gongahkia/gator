package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/journal"
)

// reviewCommand is the single human-facing review path. Work bundles retain
// artifacts, Code candidates, and verification evidence together, so legacy
// run-record and loopback-browser review are intentionally not accepted here.
func reviewCommand(arguments []string, out io.Writer) error {
	options, eligible := parseWorkReviewOptions(arguments)
	if !eligible {
		return errors.New("usage: gator work review WORK_BUNDLE|WORK_ID [--preview] [--json]")
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	bundle, found, err := resolveWorkBundle(options.reference, stateDir)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("work bundle %q was not found", options.reference)
	}
	store, err := delivery.Open(stateDir)
	if err != nil {
		return err
	}
	return reviewWorkBundleWithDelivery(out, bundle, &store, options.json, options.preview)
}
