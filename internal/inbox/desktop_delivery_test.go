package inbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDesktopDeliveryRetriesFailureWithoutChangingReceipts(t *testing.T) {
	state := t.TempDir()
	s, _ := Open(state)
	e, err := s.Add(Entry{Kind: "job", Title: "Deadline", Summary: "Changed", Status: "completed", OriginConversationID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	fail := func(Entry) error { return errors.New("private-provider-detail") }
	if attempted, err := s.SendDesktopNotification(e.ID, now, fail); !attempted || err == nil {
		t.Fatal("failed handoff accepted")
	}
	e, _ = s.Load(e.ID)
	if !e.DesktopNotifiedAt.IsZero() || e.DesktopDelivery.State != "failed" || !e.ReadAt.IsZero() || !e.DeliveredAt.IsZero() || !e.ResolvedAt.IsZero() {
		t.Fatalf("failed handoff changed receipts: %+v", e)
	}
	bytes, _ := os.ReadFile(filepath.Join(s.root, e.ID+".json"))
	if strings.Contains(string(bytes), "private-provider-detail") {
		t.Fatal("raw sender diagnostic retained")
	}
	if attempted, err := s.SendDesktopNotification(e.ID, now.Add(time.Second), fail); attempted || err != nil {
		t.Fatal("backoff bypassed", err)
	}
	s, _ = Open(state)
	if attempted, err := s.SendDesktopNotification(e.ID, now.Add(time.Minute), func(Entry) error { return nil }); !attempted || err != nil {
		t.Fatal("restart retry failed", err)
	}
	e, _ = s.Load(e.ID)
	if e.DesktopNotifiedAt.IsZero() || e.DesktopDelivery.State != "handed_off" || e.DesktopDelivery.Attempts != 2 {
		t.Fatal("successful handoff not recorded", e)
	}
	if attempted, err := s.SendDesktopNotification(e.ID, now.Add(time.Hour), fail); attempted || err != nil {
		t.Fatal("successful handoff repeated", err)
	}
}

func TestDesktopDeliveryConcurrentSendAllowsReceiptWrites(t *testing.T) {
	s, _ := Open(t.TempDir())
	e, err := s.Add(Entry{Kind: "job", Title: "Update", Status: "completed", OriginConversationID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	started, finish, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := s.SendDesktopNotification(e.ID, time.Now(), func(Entry) error { close(started); <-finish; return nil })
		done <- err
	}()
	<-started
	var calls atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.SendDesktopNotification(e.ID, time.Now(), func(Entry) error { calls.Add(1); return nil })
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 0 {
		t.Fatal("concurrent OS handoff repeated")
	}
	// Receipt mutation would deadlock here if the OS sender held the inbox lock.
	if _, err := s.Receipt("chat", e.ID, "resolve", 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	close(finish)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	e, _ = s.Load(e.ID)
	if e.ResolvedAt.IsZero() || e.DesktopNotifiedAt.IsZero() {
		t.Fatal("settlement lost concurrent receipt")
	}
}

func TestDesktopDeliveryInterruptedHandoffAndRetryLimit(t *testing.T) {
	state := t.TempDir()
	s, _ := Open(state)
	e, err := s.Add(Entry{Kind: "job", Title: "Update", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	func() {
		defer func() { _ = recover() }()
		_, _ = s.SendDesktopNotification(e.ID, now, func(Entry) error { panic("synthetic process boundary") })
	}()
	s, _ = Open(state)
	e, _ = s.Load(e.ID)
	if e.DesktopDelivery.State != "pending" || !e.DesktopNotifiedAt.IsZero() {
		t.Fatal("interrupted handoff marked successful")
	}
	for attempt := 2; attempt <= maxDesktopAttempts; attempt++ {
		at := e.DesktopDelivery.NextAttemptAt
		if sent, err := s.SendDesktopNotification(e.ID, at, func(Entry) error { return errors.New("unavailable") }); !sent || err == nil {
			t.Fatal("retry not attempted")
		}
		e, _ = s.Load(e.ID)
	}
	if e.DesktopRetryDue(now.Add(48*time.Hour)) || !strings.Contains(e.DesktopDeliveryStatus(), "5 attempts") {
		t.Fatal("retry limit ignored")
	}
}

func TestReadOrResolvedNotificationStopsRetry(t *testing.T) {
	for _, action := range []string{"read", "resolve"} {
		t.Run(action, func(t *testing.T) {
			s, _ := Open(t.TempDir())
			e, err := s.Add(Entry{Kind: "job", Title: "Update", Status: "completed", OriginConversationID: "chat"})
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			_, _ = s.SendDesktopNotification(e.ID, now, func(Entry) error { return errors.New("unavailable") })
			if _, err := s.Receipt("chat", e.ID, action, 0, now); err != nil {
				t.Fatal(err)
			}
			if sent, err := s.SendDesktopNotification(e.ID, now.Add(time.Hour), func(Entry) error { t.Fatal("read/resolved notice sent"); return nil }); sent || err != nil {
				t.Fatal("receipt failed to stop delivery", err)
			}
		})
	}
}
