package usage

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSharedAdmissionAcrossStoresAndCategories(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	if err := s.SavePolicy(Policy{DailyRequests: 2, ConcurrentRequests: 1, DailyTokens: 1000}); err != nil {
		t.Fatal(err)
	}
	first, err := s.Reserve(time.Now(), "background", "test", 600)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := Open(root)
	if _, err = other.Reserve(time.Now(), "chat", "test", 100); !errors.Is(err, ErrLimit) {
		t.Fatalf("concurrent admission: %v", err)
	}
	if err = first.Finish(50, true, false, nil); err != nil {
		t.Fatal(err)
	}
	second, err := other.Reserve(time.Now(), "routing", "test", 600)
	if err != nil {
		t.Fatal(err)
	}
	if err = second.Finish(0, false, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Reserve(time.Now(), "retrieval", "test", 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("daily admission: %v", err)
	}
	view, err := other.Snapshot(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if view.Requests != 2 || view.Tokens != 650 || view.EstimatedRequests != 1 || view.ActiveRequests != 0 || view.KnownCostUSD != nil || view.Categories["background"] != 1 || view.Categories["routing"] != 1 {
		t.Fatalf("incorrect aggregate: %#v", view)
	}
}
func TestConcurrentReservationsCannotOverspendDailyLimit(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	if err := s.SavePolicy(Policy{DailyRequests: 1}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var accepted int
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other, _ := Open(root)
			r, err := other.Reserve(time.Now(), "chat", "test", 10)
			if err == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
				if err = r.Finish(3, true, false, nil); err != nil {
					t.Error(err)
				}
			} else if !errors.Is(err, ErrLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted != 1 {
		t.Fatalf("accepted %d requests under a cap of 1", accepted)
	}
}
