package main

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workrun"
)

// This helper has no UI worker, model/provider setup or retention maintenance.
func companionInspection(args []string, localGrant bool, out io.Writer) error {
	root, err := state.ResolveDir("")
	if err != nil {
		return err
	}
	release, err := state.Guard(root, false)
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if localGrant {
		seconds, err := strconv.Atoi(args[2])
		if err != nil {
			return err
		}
		return workrun.GrantWorkInspection(ctx, root, args[0], args[1], seconds)
	}
	response, err := workrun.InspectOwnedWork(ctx, root, args[0], args[1], args[2])
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(response)
}
