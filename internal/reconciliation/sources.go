// Package reconciliation checks evidence identities for reviewable conflicts.
// It does not decide which source is true or automatically rewrite knowledge.
package reconciliation

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/knowledge"
)

type Source struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Quote    string `json:"quote"`
}

func Validate(sources []Source) error {
	if len(sources) < 2 || len(sources) > 8 {
		return errors.New("a conflict needs 2 through 8 distinct sources")
	}
	seen := map[string]bool{}
	for _, source := range sources {
		if source.Kind != "note" && source.Kind != "plan" {
			return errors.New("conflict sources must be notes or plans")
		}
		if len(source.ID) != 29 || strings.ContainsAny(source.ID, "/\\\x00\r\n") || source.Revision == "" || len(source.Revision) > 64 || strings.TrimSpace(source.Quote) == "" || len(source.Quote) > 1024 || !utf8.ValidString(source.Quote) || strings.ContainsRune(source.Quote, 0) {
			return errors.New("invalid conflict source identity or quote")
		}
		key := source.Kind + ":" + source.ID
		if seen[key] {
			return errors.New("conflict sources must be distinct")
		}
		seen[key] = true
	}
	return nil
}

func Check(goals goal.Store, notes knowledge.Store, sources []Source, scope string, allowGlobal bool, excluded []string) error {
	if err := Validate(sources); err != nil {
		return err
	}
	for _, source := range sources {
		var revision, content string
		switch source.Kind {
		case "note":
			if slices.Contains(excluded, source.ID) {
				return errors.New("conflict source is excluded from this run")
			}
			note, err := notes.LoadNote(source.ID)
			if err != nil {
				return err
			}
			if note.GoalID != "" && note.GoalID != scope {
				return errors.New("conflict note is outside this conversation")
			}
			revision, content = note.Revision(), note.Title+"\n"+note.Body
		case "plan":
			if source.ID != scope && !(scope == "" && allowGlobal) {
				return errors.New("conflict plan is outside this conversation")
			}
			plan, err := goals.LoadPlan(source.ID)
			if err != nil {
				return err
			}
			data, err := json.Marshal(plan)
			if err != nil {
				return err
			}
			revision, content = strconv.Itoa(plan.Revision), string(data)
		}
		if source.Revision != revision {
			return fmt.Errorf("conflict source %s changed; inspect it again", source.ID)
		}
		if !strings.Contains(content, source.Quote) {
			return fmt.Errorf("conflict quote is not present in %s", source.ID)
		}
	}
	return nil
}
