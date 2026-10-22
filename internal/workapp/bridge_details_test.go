package workapp

import "testing"

func TestBridgeWorkDetailsSeparatesSelectionsFromRetainedUse(t *testing.T) {
	model := New(Config{CurrentFolder: t.TempDir()})
	model.options.Mode = "auto"
	model.options.Attachments = []string{"notes.md"}
	model.options.ConnectorIDs = []string{"next-request"}
	model.snapshot = "snapshot-one"
	model.lastBundle = BundleSummary{Path: "/retained/run", Status: "failed", Connections: []ConnectionUse{{ID: "previous-run", Operations: []string{"fetch"}}}, Sources: []string{"source-one"}}
	state := model.BridgeState()
	if state.WorkDetails.Mode != "auto" || state.WorkDetails.SnapshotID != "snapshot-one" || state.WorkDetails.ConnectorIDs[0] != "next-request" || state.WorkDetails.LatestBundle.Connections[0].ID != "previous-run" || state.WorkDetails.LatestBundle.Status != "failed" {
		t.Fatalf("details = %#v", state.WorkDetails)
	}
	state.WorkDetails.ConnectorIDs[0] = "mutated"
	state.WorkDetails.Attachments[0] = "mutated"
	state.WorkDetails.LatestBundle.Connections[0].Operations[0] = "mutated"
	state.WorkDetails.LatestBundle.Sources[0] = "mutated"
	if model.options.ConnectorIDs[0] != "next-request" || model.options.Attachments[0] != "notes.md" || model.lastBundle.Connections[0].Operations[0] != "fetch" || model.lastBundle.Sources[0] != "source-one" {
		t.Fatal("display projection mutated authoritative selections or evidence")
	}
	next, _ := model.SubmitText("/new")
	if next.(Model).BridgeState().WorkDetails.LatestBundle != nil {
		t.Fatal("a new conversation retained the previous run's evidence")
	}
}
