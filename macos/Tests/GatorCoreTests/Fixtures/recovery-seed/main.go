// Disposable test setup only. Retain a real sealed unknown-send bundle using Go stores.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/workspace"
	"os"
	"time"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	if len(os.Args) != 3 {
		panic("disposable workspace and local service URL required")
	}
	store, err := config.DefaultStore()
	must(err)
	settings, err := store.Load()
	must(err)
	settings.Connectors = []connector.Descriptor{{Version: connector.DescriptorVersion, ID: "google", Name: "Local receipt fixture", Kind: connector.KindGoogle, Resource: os.Args[2], Authentication: connector.AuthNone}}
	must(store.Save(settings))
	registry, err := connector.NewRegistry(settings.Connectors)
	must(err)
	prepared, err := (connector.Runtime{Registry: registry}).PrepareAction("google", "gmail_send", json.RawMessage(`{"payload":{"from":"me@example.com","to":"venue@example.com","subject":"Uncertain venue update","body":"PRIVATE_BODY_MUST_NOT_ENTER_RECOVERY"}}`))
	must(err)
	stateDir := os.Getenv("GATOR_STATE_DIR")
	work, err := workspace.CreateWork(os.Args[1], stateDir, "receipt-send", time.Now())
	must(err)
	contract := artifact.DefaultContract("report.md")
	contract.ExternalActions = action.Approve
	must(artifact.WriteText(work.Output, contract, "report.md", "Partial preparation retained; external send outcome unknown.\n"))
	manifest, err := artifact.Seal(work.Output, contract, artifact.SealOptions{RunID: work.ID, Objective: "Uncertain venue update", Source: work.Source, Actions: []action.Record{{Proposal: prepared.Proposal, Status: action.Unknown, Error: "Controlled interruption: send outcome unconfirmed"}}, StartedAt: work.CreatedAt, FinishedAt: work.CreatedAt})
	must(err)
	must(artifact.WriteManifest(work.ManifestPath, manifest))
	sessions, err := worksession.Open(stateDir)
	must(err)
	conversation, err := sessions.Create("Uncertain venue update", work.Source.Path(), "snap-recovery", time.Now())
	must(err)
	_, err = sessions.AddRevision(conversation.ID, worksession.Revision{SnapshotID: "snap-recovery", Objective: "Uncertain venue update", BundlePath: work.Path, Status: "failed", FinalText: "Partial preparation retained. Send outcome unknown; do not resend."})
	must(err)
	fmt.Println(conversation.ID)
}
