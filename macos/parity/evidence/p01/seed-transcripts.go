// Desktop-only disposable fixture. No provider or execution admission is used.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/worksession"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	root := os.Args[1]
	if !strings.HasPrefix(root, "/tmp/gator-p01-") {
		panic("disposable P01 profile required")
	}
	check(os.MkdirAll(filepath.Join(root, "workspace"), 0700))
	store, err := worksession.Open(filepath.Join(root, "state"))
	check(err)
	var ids []string
	for _, label := range []string{"A", "B"} {
		conversation, err := store.CreateChat(filepath.Join(root, "workspace"), time.Now())
		check(err)
		ids = append(ids, conversation.ID)
		count := 30
		if label == "A" {
			count = 105
		}
		for i := 1; i <= count; i++ {
			now := time.Now()
			turn, err := store.AdmitDialogue(conversation.ID, fmt.Sprintf("gator-p01-fixture-%s-%03d", label, i), fmt.Sprintf("History question %s-%03d", label, i), json.RawMessage(`{"version":1}`), now)
			check(err)
			turn.State = "routing"
			turn.UpdatedAt = time.Now()
			turn, err = store.UpdateDialogue(turn, turn.Revision)
			check(err)
			turn.State = "answered"
			turn.Response = fmt.Sprintf("## Retained note %s-%03d\n\nSearchable café and 東京. Needle in a retained reply.\n\n- Parent item\n  7. Nested **bold** item with `code`\n\n", label, i) + strings.Repeat("Ordinary public fixture prose for reading position verification. ", 5)
			turn.UpdatedAt = time.Now()
			_, err = store.UpdateDialogue(turn, turn.Revision)
			check(err)
		}
	}
	check(store.SetActive(ids[0]))
	check(json.NewEncoder(os.Stdout).Encode(ids))
}
