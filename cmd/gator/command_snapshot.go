package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/worksession"
)

func snapshotCommand(arguments []string, out io.Writer) error {
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	if len(arguments) == 0 || arguments[0] == "list" {
		manifests, err := snapshot.List(stateDir)
		if err != nil {
			return err
		}
		for _, manifest := range manifests {
			fmt.Fprintf(out, "%s\t%d files\t%d bytes\t%s\n", manifest.ID, manifest.Files, manifest.Bytes, manifest.SourcePath)
		}
		return nil
	}
	switch arguments[0] {
	case "show":
		if len(arguments) != 2 {
			return errors.New("usage: gator snapshot show ID")
		}
		manifest, err := snapshot.Open(stateDir, arguments[1])
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(manifest)
	case "gc":
		if len(arguments) != 2 || arguments[1] != "--yes" {
			return errors.New("snapshot gc requires --yes")
		}
		sessions, err := worksession.Open(stateDir)
		if err != nil {
			return err
		}
		conversations, err := sessions.List(0)
		if err != nil {
			return err
		}
		referenced := make(map[string]struct{})
		for _, conversation := range conversations {
			referenced[conversation.SnapshotID] = struct{}{}
			revisions, err := sessions.Revisions(conversation.ID)
			if err != nil {
				return err
			}
			for _, revision := range revisions {
				referenced[revision.SnapshotID] = struct{}{}
			}
		}
		snapshots, blobs, err := snapshot.GC(stateDir, referenced)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Removed %d unreferenced snapshots and %d unreachable blobs. This cannot be undone from Gator.\n", snapshots, blobs)
		return err
	default:
		return fmt.Errorf("unknown snapshot command %q", arguments[0])
	}
}
