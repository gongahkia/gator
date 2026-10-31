package main

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/gongahkia/gator/internal/companionaccess"
	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/executiontarget/external"
	"github.com/gongahkia/gator/internal/executiontarget/tmux"
	"github.com/gongahkia/gator/internal/state"
)

// Short-lived host helper: no Backend worker, maintenance, provider or Work owner.
func companionExternal(args []string, local bool, input io.Reader, out io.Writer) error {
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
	store, err := external.Open(root)
	if err != nil {
		return err
	}
	encode := func(v any) error { return json.NewEncoder(out).Encode(v) }
	if local {
		if len(args) == 4 && args[0] == "candidates" && args[1] == "--reviewed-read-config" {
			config, e := tmux.AcceptReadConfig(tmux.Config{Executable: args[2], Socket: args[3]}, executiontarget.Actor{ID: "local-profile-owner", Kind: executiontarget.Human, Origin: executiontarget.Local})
			if e != nil {
				return e
			}
			c, e := tmux.New(config)
			if e != nil {
				return e
			}
			v, e := c.Candidates(ctx)
			if e != nil {
				return e
			}
			return encode(v)
		}
		if len(args) == 6 && args[0] == "enroll" && args[1] == "--reviewed-read-config" {
			config, e := tmux.AcceptReadConfig(tmux.Config{Executable: args[2], Socket: args[3]}, executiontarget.Actor{ID: "local-profile-owner", Kind: executiontarget.Human, Origin: executiontarget.Local})
			if e != nil {
				return e
			}
			v, e := store.Enroll(ctx, config, args[4], args[5])
			if e != nil {
				return e
			}
			return encode(v.Target)
		}
		if len(args) == 1 && args[0] == "list" {
			v, e := store.LocalStatus(ctx)
			if e != nil {
				return e
			}
			return encode(v)
		}
		if len(args) == 2 && args[0] == "unreview" {
			return store.Unreview(args[1])
		}
		if len(args) == 2 && args[0] == "revoke" {
			return store.Revoke(args[1])
		}
		if len(args) == 2 && args[0] == "grant" {
			var v struct {
				Authority companionaccess.Reference `json:"authority"`
				Seconds   int                       `json:"seconds"`
			}
			d := json.NewDecoder(io.LimitReader(input, 8193))
			d.DisallowUnknownFields()
			if d.Decode(&v) != nil || d.Decode(new(any)) != io.EOF {
				return external.ErrDenied
			}
			return store.Grant(ctx, args[1], v.Authority, v.Seconds)
		}
		return external.ErrDenied
	}
	if len(args) != 4 {
		return external.ErrDenied
	}
	actor := executiontarget.Actor{ID: args[0], Kind: executiontarget.Human, Origin: executiontarget.Remote}
	if args[1] == "list" && args[2] == "" && args[3] == "" {
		v, e := store.List(ctx, actor)
		if e != nil {
			return e
		}
		return encode(v)
	}
	if args[1] != "metadata" && args[1] != "snapshot" {
		return external.ErrDenied
	}
	v, e := store.Read(ctx, actor, args[2], args[3], 100, args[1] == "snapshot")
	if e != nil {
		return encode(external.Response{Targets: []external.Snapshot{}, Unavailable: true, Reason: "Target unavailable or access withdrawn."})
	}
	return encode(external.Response{Targets: []external.Snapshot{v}})
}
