package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/privatefs"
)

func TestPublicationNeverExposesPartialRecordsToConcurrentReaders(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	failures := make(chan error, 4)
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := store.List("", false); err != nil {
					failures <- err
					return
				}
			}
		}()
	}
	for range 100 {
		record, err := store.Add("note", strings.Repeat("complete evidence ", 200), "", "fixture", time.Time{})
		if err != nil {
			t.Error(err)
			break
		}
		if _, err := store.Correct(record.ID, "Corrected complete evidence", "fixture"); err != nil {
			t.Error(err)
			break
		}
	}
	close(stop)
	readers.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("reader saw incomplete publication: %v", err)
	}
}

func TestExclusivePublicationPreservesExistingIdentityAndIgnoresInterruptedStaging(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("fact", "Original fact", "", "fixture", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(store.path(record.ID))
	if err != nil {
		t.Fatal(err)
	}
	record.Text = "Must not replace the original"
	if err := store.write(record, true); !errors.Is(err, os.ErrExist) {
		t.Fatalf("exclusive publication = %v", err)
	}
	current, err := os.ReadFile(store.path(record.ID))
	if err != nil || string(current) != string(original) {
		t.Fatal("ID collision changed the original bytes", err)
	}
	if err := os.WriteFile(filepath.Join(store.root, ".memory-interrupted"), []byte(`{"text":`), 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(filepath.Dir(filepath.Dir(store.root)))
	if err != nil {
		t.Fatal(err)
	}
	records, err := reopened.List("", false)
	if err != nil || len(records) != 1 || records[0].Text != "Original fact" {
		t.Fatalf("interrupted staging entered recall: %#v %v", records, err)
	}
	if err := privatefs.Check(store.path(record.ID)); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryRejectsLinkedOwnedDirectories(t *testing.T) {
	for _, component := range []string{"gator", "memory"} {
		t.Run(component, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			path := filepath.Join(root, "gator")
			if component == "memory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(path, "memory")
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Skipf("symlink creation unavailable: %v", err)
			}
			if _, err := Open(root); err == nil {
				t.Fatal("followed an owned-directory symlink")
			}
		})
	}
}
