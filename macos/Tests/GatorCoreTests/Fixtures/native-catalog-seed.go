// Explicit disposable fixture only. All retained state uses canonical Go APIs.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/worksession"
)

func checked(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	root := os.Args[1]
	if !strings.HasPrefix(filepath.Base(root), "gator-native-test-") && !strings.HasPrefix(root, "/tmp/gator-p02-") {
		panic("disposable fixture root required")
	}
	s, err := worksession.Open(filepath.Join(root, "state"))
	checked(err)
	if len(os.Args) > 2 && os.Args[2] == "fallback" {
		path := filepath.Join(root, "state", "gator", "cache")
		checked(os.RemoveAll(path))
		checked(os.WriteFile(path, []byte("fixture unavailable cache"), 0600))
		fmt.Println(`{"fallback":true}`)
		return
	}
	if len(os.Args) > 3 && os.Args[2] == "rename" {
		c, err := s.Load(os.Args[3])
		checked(err)
		c, err = s.OrganizeConversation(worksession.ConversationChange{ID: c.ID, Version: worksession.ConversationDisplayVersion(c), Action: "rename", Title: os.Args[4]})
		checked(err)
		checked(json.NewEncoder(os.Stdout).Encode(c))
		return
	}
	if len(os.Args) > 3 && os.Args[2] == "read" {
		c, err := s.Load(os.Args[3])
		checked(err)
		checked(json.NewEncoder(os.Stdout).Encode(map[string]any{"id": c.ID, "title": c.Title, "archived": c.Archived, "pinned": c.Pinned, "version": worksession.ConversationDisplayVersion(c), "head_revision_id": c.HeadRevision}))
		return
	}
	count, err := strconv.Atoi(os.Args[2])
	checked(err)
	if count < 1 || count > 11000 {
		panic("bounded fixture size required")
	}
	source := filepath.Join(root, "workspace")
	checked(os.MkdirAll(source, 0700))
	ids := []string{}
	at := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		c, err := s.CreateChat(source, at.Add(time.Duration(i)*time.Minute))
		checked(err)
		c, err = s.OrganizeConversation(worksession.ConversationChange{ID: c.ID, Version: worksession.ConversationDisplayVersion(c), Action: "rename", Title: fmt.Sprintf("Retained topic %04d", i)})
		checked(err)
		if i%7 == 0 {
			c, err = s.OrganizeConversation(worksession.ConversationChange{ID: c.ID, Version: worksession.ConversationDisplayVersion(c), Action: "archive"})
			checked(err)
		}
		if i%17 == 0 {
			c, err = s.OrganizeConversation(worksession.ConversationChange{ID: c.ID, Version: worksession.ConversationDisplayVersion(c), Action: "pin"})
			checked(err)
		}
		if i == 0 {
			_, err = s.AddRevision(c.ID, worksession.Revision{ID: "rev-catalog-fixture", SnapshotID: "snap-fixture", Objective: "Retained public objective", FinalText: "The jade decision is in retained Work text.", Status: "completed", BundlePath: "/fixture/output", CreatedAt: at})
			checked(err)
		}
		if i == 5 || i == count-1 {
			for j := 1; j <= 35; j++ {
				turn, err := s.AdmitDialogue(c.ID, fmt.Sprintf("catalog-dialogue-%04d-%04d", i, j), fmt.Sprintf("Question topic %04d note %02d", i, j), json.RawMessage(`{"version":1}`), at.Add(time.Duration(i)*time.Minute+time.Duration(j)*time.Second))
				checked(err)
				turn.State = "routing"
				turn, err = s.UpdateDialogue(turn, turn.Revision)
				checked(err)
				turn.State = "answered"
				turn.Response = fmt.Sprintf("## Retained note %04d-%02d\n\nPublic café response.\n\n- Parent\n  7. Nested **bold** and `code`\n\n", i, j) + strings.Repeat("Read this retained fixture text. ", 6)
				_, err = s.UpdateDialogue(turn, turn.Revision)
				checked(err)
			}
		}
		ids = append(ids, c.ID)
	}
	checked(s.SetActive(ids[len(ids)-1]))
	checked(json.NewEncoder(os.Stdout).Encode(ids))
}
