// Disposable visual/performance fixture only. Uses the existing durable Go API.
package main

import (
	"fmt"
	"github.com/gongahkia/gator/internal/worksession"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) != 4 {
		panic("state root, existing controlled conversation, revision count")
	}
	s, err := worksession.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	base, err := s.Load(os.Args[2])
	if err != nil {
		panic(err)
	}
	head, err := s.LoadRevision(base.ID, base.HeadRevision)
	if err != nil {
		panic(err)
	}
	count, err := strconv.Atoi(os.Args[3])
	if err != nil || count < 1 || count > 500 {
		panic("bounded count required")
	}
	next, err := s.Create("Long conversation · controlled fixture", base.SourcePath, base.SnapshotID, time.Now())
	if err != nil {
		panic(err)
	}
	parent := ""
	for i := 0; i < count; i++ {
		r := worksession.Revision{SnapshotID: base.SnapshotID, ParentRevisionID: parent, Objective: fmt.Sprintf("Controlled question %d", i+1), BundlePath: head.BundlePath, Status: "completed", FinalText: fmt.Sprintf("Controlled response %d. This retained test history is synthetic; no real work or external effect is claimed.\n\nThe conversation stays readable while its exact evidence remains inspectable.", i+1), CreatedAt: time.Now().Add(time.Duration(i) * time.Millisecond)}
		next, err = s.AddRevision(next.ID, r)
		if err != nil {
			panic(err)
		}
		parent = next.HeadRevision
	}
	fmt.Println(next.ID)
}
