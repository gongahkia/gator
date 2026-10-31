package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
)

func decodeCompanionControl(in io.Reader, v any) error {
	d := json.NewDecoder(io.LimitReader(in, 28*1024+1))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("invalid control input")
	}
	return nil
}
func companionControl(mode string, in io.Reader, out io.Writer) error {
	var input struct {
		Actor  string               `json:"actor"`
		Action workrun.RemoteAction `json:"action"`
		RunID  string               `json:"run_id"`
		Target string               `json:"target"`
		ID     string               `json:"id"`
	}
	if err := decodeCompanionControl(in, &input); err != nil {
		return err
	}
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
	var reply workrun.ControlReply
	if mode == "receipt" {
		store, e := actionlog.Open(root)
		if e != nil {
			return e
		}
		r, e := store.LookupControl(input.RunID, input.Target, input.ID)
		if e != nil || r.Actor.ID != input.Actor {
			return errors.New("receipt unavailable")
		}
		reply.Receipt = &r
		// This observation never changes the receipt's delivery classification.
		// A stopped Work lease does not prove every descendant program exited.
		if history, e := workhistory.Open(root); e == nil {
			if live, e := history.LiveStatus(r.RunID); e == nil && live == "stopped" {
				reply.Observation = "work_lease_stopped"
			}
		}
	} else {
		reply, err = workrun.RouteRemoteControl(ctx, root, input.Actor, mode, input.Action)
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(out).Encode(reply)
}
func companionControlGrant(run string, in io.Reader, out io.Writer) error {
	var input workrun.ControlAccess
	if err := decodeCompanionControl(in, &input); err != nil {
		return err
	}
	root, err := state.ResolveDir("")
	if err != nil {
		return err
	}
	release, err := state.Guard(root, false)
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return workrun.GrantRemoteControl(ctx, root, run, input)
}
