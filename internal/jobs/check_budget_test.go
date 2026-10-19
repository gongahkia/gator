package jobs

import (
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
)

func TestDailyCheckReservationIsAtomicAcrossConcurrentStarts(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(Definition{Name: "Bounded check", SourcePath: t.TempDir(), RefreshSnapshot: true, Objective: "Review project", Schedule: "0 * * * *", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxChecksPerDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	one, err := store.Begin(definition, now)
	if err != nil {
		t.Fatal(err)
	}
	two, err := store.Begin(definition, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan bool, 2)
	failures := make(chan error, 2)
	var group sync.WaitGroup
	for _, attempt := range []Attempt{one, two} {
		group.Add(1)
		go func(a Attempt) {
			defer group.Done()
			ok, err := store.ReserveCheck(definition, a, now)
			results <- ok
			failures <- err
		}(attempt)
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	reserved := 0
	for ok := range results {
		if ok {
			reserved++
		}
	}
	if reserved != 1 {
		t.Fatalf("reserved %d checks under a limit of one", reserved)
	}
	count, err := store.DailyChecks(definition.ID, now)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
