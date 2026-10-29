package learning

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func addFailure(t *testing.T, s Store, work string, scope *Scope) {
	t.Helper()
	if _, err := s.RecordObservation(ObservationInput{WorkID: work, Signal: VerificationFailed, Scope: scope, EvidenceRefs: []string{"gator/work-history/" + work + ".json"}}); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceRequiresIndependentEvidenceAndPreservesReviewedState(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	for i := 0; i < 5; i++ {
		addFailure(t, s, "work-one", nil)
	}
	first, err := s.Maintain(context.Background())
	if err != nil || len(first.Candidates) != 0 {
		t.Fatal("one run counted as independent failures", first, err)
	}
	addFailure(t, s, "work-two", nil)
	project := Scope{Kind: Project, Value: t.TempDir()}
	addFailure(t, s, "work-project", &project)
	second, err := s.Maintain(context.Background())
	if err != nil || len(second.Candidates) != 0 {
		t.Fatal("scopes mixed", second, err)
	}
	addFailure(t, s, "work-three", nil)
	report, err := s.Maintain(context.Background())
	if err != nil || len(report.Candidates) != 1 {
		t.Fatal(report, err)
	}
	record, err := s.Load(report.Candidates[0])
	if err != nil || record.Status != Candidate || record.Confidence != 0 || len(record.Provenance.WorkIDs) != 3 || len(record.Provenance.EvidenceRefs) != 3 {
		t.Fatal(record, err)
	}
	if active, err := s.Projection(Context{}); err != nil || len(active) != 0 {
		t.Fatal("unreviewed proposal became guidance", active, err)
	}
	if _, err := s.Reject(record.ID); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path(record.ID))
	reopened, _ := Open(dir)
	addFailure(t, reopened, "work-four", nil)
	report, err = reopened.Maintain(context.Background())
	after, _ := os.ReadFile(s.path(record.ID))
	if err != nil || len(report.Candidates) != 0 || report.Existing != 1 || string(before) != string(after) {
		t.Fatal("maintenance changed rejected state", report, err)
	}
	status, err := reopened.MaintenanceStatus()
	if err != nil || status.Existing != 1 || status.FinishedAt.IsZero() {
		t.Fatal("maintenance status missing", status, err)
	}
}

func TestConcurrentMaintenancePublishesOneCompleteCandidate(t *testing.T) {
	s, _ := Open(t.TempDir())
	for i := 0; i < 3; i++ {
		addFailure(t, s, fmt.Sprintf("work-%d", i), nil)
	}
	var group sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() { defer group.Done(); _, err := s.Maintain(context.Background()); errors <- err }()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	records, err := s.List()
	if err != nil || len(records) != 1 {
		t.Fatal("duplicate or partial candidates", records, err)
	}
	if _, err := s.Enable(records[0].ID); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path(records[0].ID))
	if _, err := s.Maintain(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.path(records[0].ID))
	if string(before) != string(after) {
		t.Fatal("maintenance changed approved state")
	}
}

func TestMaintenanceReportsBoundsCorruptionAndCancellation(t *testing.T) {
	s, _ := Open(t.TempDir())
	root, err := s.observationsRoot()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maintenanceScanLimit+2; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("bad-%d.json", i)), []byte("{"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := s.Maintain(context.Background())
	if err != nil || !report.Truncated || report.Scanned != maintenanceScanLimit || report.Invalid != maintenanceScanLimit || len(report.Candidates) != 0 {
		t.Fatal("unbounded or hidden corruption", report, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err = s.Maintain(ctx)
	if err == nil || report.Scanned != 0 {
		t.Fatal("ignored cancellation", report, err)
	}
	status, err := s.MaintenanceStatus()
	if err != nil || status.Error == "" {
		t.Fatal("hidden maintenance failure", status, err)
	}
}

func TestMaintenanceDoesNotUseOldOrUnlinkedObservations(t *testing.T) {
	s, _ := Open(t.TempDir())
	old := time.Now().AddDate(0, 0, -31)
	s.now = func() time.Time { return old }
	for i := 0; i < 3; i++ {
		addFailure(t, s, fmt.Sprintf("work-old-%d", i), nil)
	}
	s.now = time.Now
	for i := 0; i < 3; i++ {
		if _, err := s.RecordObservation(ObservationInput{Signal: WorkFailed, WorkID: fmt.Sprintf("work-unlinked-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := s.Maintain(context.Background())
	if err != nil || len(report.Candidates) != 0 {
		t.Fatal("old/unlinked evidence proposed guidance", report, err)
	}
}
