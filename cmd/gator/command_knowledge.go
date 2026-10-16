package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/state"
)

const noteUsage = `Gator notes — private, searchable Markdown knowledge

  gator note add --title TITLE [--goal ID] [--source-ref REF ...] [--file PATH | --body TEXT]
  gator note list [--goal ID]
  gator note search [--goal ID] QUERY
  gator note show|path ID
  gator note edit ID [--title TITLE] [--file PATH | --body TEXT]
  gator note remove ID --yes`

const skillUsage = `Gator skills — user-owned text procedures

  gator skill create NAME --description WHEN_TO_USE [--file PATH | --body TEXT]
  gator skill list [--all]
  gator skill show|path NAME
  gator skill edit NAME [--description WHEN_TO_USE] [--file PATH | --body TEXT]
  gator skill enable|disable NAME
  gator skill remove NAME --yes

Skills are Markdown guidance. Their text cannot grant tools, run commands,
or change sandbox policy.`

func knowledgeStore() (knowledge.Store, string, error) {
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return knowledge.Store{}, "", err
	}
	store, err := knowledge.Open(stateDir)
	return store, stateDir, err
}

func noteCommand(args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArgument(args[0]) {
		_, err := fmt.Fprintln(out, noteUsage)
		return err
	}
	store, stateDir, err := knowledgeStore()
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		flags := flag.NewFlagSet("note add", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		title := flags.String("title", "", "note title")
		goalID := flags.String("goal", "", "goal ID")
		file := flags.String("file", "", "Markdown file")
		body := flags.String("body", "", "note body")
		var sourceRefs stringFlags
		flags.Var(&sourceRefs, "source-ref", "document, citation, or Work reference")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("usage: gator note add --title TITLE [--goal ID] [--file PATH | --body TEXT]")
		}
		if *goalID != "" {
			goals, err := goal.Open(stateDir)
			if err != nil {
				return err
			}
			if _, err := goals.Load(*goalID); err != nil {
				return err
			}
		}
		content, err := knowledgeBody(*file, *body, 64*1024)
		if err != nil {
			return err
		}
		d, err := store.AddNoteWithRefs(*title, content, *goalID, sourceRefs)
		if err != nil {
			return err
		}
		path, _ := store.NotePath(d.ID)
		_, err = fmt.Fprintf(out, "%s  %s\n%s\n", d.ID, d.Title, path)
		return err
	case "list":
		flags := flag.NewFlagSet("note list", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		goalID := flags.String("goal", "", "goal ID")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("usage: gator note list [--goal ID]")
		}
		items, err := store.ListNotes(*goalID)
		if err != nil {
			return err
		}
		for _, d := range items {
			if _, err := fmt.Fprintf(out, "%s  %-29s %s\n", d.ID, valueOrDash(d.GoalID), d.Title); err != nil {
				return err
			}
		}
		return nil
	case "search":
		flags := flag.NewFlagSet("note search", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		goalID := flags.String("goal", "", "goal ID")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		items, err := store.SearchNotes(strings.Join(flags.Args(), " "), *goalID, 30)
		if err != nil {
			return err
		}
		for _, d := range items {
			if _, err := fmt.Fprintf(out, "%s  %s\n", d.ID, d.Title); err != nil {
				return err
			}
		}
		return nil
	case "show", "path":
		if len(args) != 2 {
			return fmt.Errorf("usage: gator note %s ID", args[0])
		}
		if args[0] == "path" {
			path, err := store.NotePath(args[1])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, path)
			return err
		}
		d, err := store.LoadNote(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "# %s\nID: %s\nGoal: %s\nSources: %s\nUpdated: %s\n\n%s\n", d.Title, d.ID, valueOrDash(d.GoalID), valueOrDash(strings.Join(d.SourceRefs, ", ")), d.UpdatedAt.Format("2006-01-02"), d.Body)
		return err
	case "edit":
		if len(args) < 2 {
			return errors.New("usage: gator note edit ID [--title TITLE] [--file PATH | --body TEXT]")
		}
		prior, err := store.LoadNote(args[1])
		if err != nil {
			return err
		}
		flags := flag.NewFlagSet("note edit", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		title := flags.String("title", prior.Title, "note title")
		file := flags.String("file", "", "Markdown file")
		body := flags.String("body", "", "note body")
		var sourceRefs stringFlags
		flags.Var(&sourceRefs, "source-ref", "replace source references")
		clearRefs := flags.Bool("clear-source-refs", false, "remove source references")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("unexpected note edit arguments")
		}
		content := prior.Body
		if *file != "" || *body != "" {
			content, err = knowledgeBody(*file, *body, 64*1024)
			if err != nil {
				return err
			}
		}
		refs := prior.SourceRefs
		if len(sourceRefs) > 0 {
			refs = sourceRefs
		}
		if *clearRefs {
			if len(sourceRefs) > 0 {
				return errors.New("cannot combine --source-ref and --clear-source-refs")
			}
			refs = nil
		}
		d, err := store.UpdateNoteWithRefs(args[1], *title, content, refs)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Updated %s.\n", d.ID)
		return err
	case "remove":
		if len(args) != 3 || args[2] != "--yes" {
			return errors.New("usage: gator note remove ID --yes")
		}
		if err := store.RemoveNote(args[1]); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Removed %s.\n", args[1])
		return err
	default:
		return fmt.Errorf("unknown note command %q", args[0])
	}
}

func skillCommand(args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArgument(args[0]) {
		_, err := fmt.Fprintln(out, skillUsage)
		return err
	}
	store, _, err := knowledgeStore()
	if err != nil {
		return err
	}
	switch args[0] {
	case "create", "edit":
		if len(args) < 2 {
			return fmt.Errorf("usage: gator skill %s NAME --description TEXT [--file PATH | --body TEXT]", args[0])
		}
		var prior knowledge.Document
		if args[0] == "edit" {
			prior, err = store.LoadSkill(args[1])
			if err != nil {
				return err
			}
		}
		flags := flag.NewFlagSet("skill "+args[0], flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		description := flags.String("description", prior.Description, "when to use this skill")
		file := flags.String("file", "", "Markdown file")
		body := flags.String("body", "", "skill body")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("unexpected skill arguments")
		}
		content := prior.Body
		if args[0] == "create" || *file != "" || *body != "" {
			content, err = knowledgeBody(*file, *body, 32*1024)
			if err != nil {
				return err
			}
		}
		var d knowledge.Document
		if args[0] == "create" {
			d, err = store.AddSkill(args[1], *description, content)
		} else {
			d, err = store.UpdateSkill(args[1], *description, content)
		}
		if err != nil {
			return err
		}
		path, _ := store.SkillPath(d.ID)
		_, err = fmt.Fprintf(out, "%s  %s\n", d.ID, path)
		return err
	case "list":
		all := len(args) == 2 && args[1] == "--all"
		if len(args) > 1 && !all {
			return errors.New("usage: gator skill list [--all]")
		}
		items, err := store.ListSkills(!all)
		if err != nil {
			return err
		}
		for _, d := range items {
			state := "disabled"
			if d.Enabled {
				state = "enabled"
			}
			if _, err := fmt.Fprintf(out, "%s  %-8s %s\n", d.ID, state, d.Description); err != nil {
				return err
			}
		}
		return nil
	case "show", "path":
		if len(args) != 2 {
			return fmt.Errorf("usage: gator skill %s NAME", args[0])
		}
		if args[0] == "path" {
			path, err := store.SkillPath(args[1])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, path)
			return err
		}
		d, err := store.LoadSkill(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "# %s\nDescription: %s\nEnabled: %t\nUpdated: %s\n\n%s\n", d.Title, d.Description, d.Enabled, d.UpdatedAt.Format("2006-01-02"), d.Body)
		return err
	case "enable", "disable":
		if len(args) != 2 {
			return fmt.Errorf("usage: gator skill %s NAME", args[0])
		}
		d, err := store.SetSkillEnabled(args[1], args[0] == "enable")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s  enabled=%t\n", d.ID, d.Enabled)
		return err
	case "remove":
		if len(args) != 3 || args[2] != "--yes" {
			return errors.New("usage: gator skill remove NAME --yes")
		}
		if err := store.RemoveSkill(args[1]); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Removed %s.\n", args[1])
		return err
	default:
		return fmt.Errorf("unknown skill command %q", args[0])
	}
}

func knowledgeBody(path, body string, maxBytes int64) (string, error) {
	if path != "" && body != "" {
		return "", errors.New("use either --file or --body")
	}
	if path == "" {
		if strings.TrimSpace(body) == "" {
			return "", errors.New("provide --file PATH or --body TEXT")
		}
		return body, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return "", errors.New("input must be a bounded regular file")
	}
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	opened, err := input.Stat()
	if err != nil {
		return "", err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Size() > maxBytes {
		return "", errors.New("input file changed during read")
	}
	data, err := io.ReadAll(io.LimitReader(input, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxBytes {
		return "", errors.New("input file exceeds size limit")
	}
	return string(data), nil
}
