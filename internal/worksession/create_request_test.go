package worksession

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCreateChatOnceConcurrentProcesses(t *testing.T) {
	if root := os.Getenv("GATOR_TEST_CREATE_CHAT_ROOT"); root != "" {
		store, err := Open(root)
		if err != nil {
			panic(err)
		}
		chat, err := store.CreateChatOnce(root, "same-client-request-1234", time.Now())
		if err != nil {
			panic(err)
		}
		os.Stdout.WriteString(chat.ID)
		os.Exit(0)
	}
	root := t.TempDir()
	var wait sync.WaitGroup
	ids, failures := make(chan string, 4), make(chan error, 4)
	for range 4 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			command := exec.Command(os.Args[0], "-test.run=^TestCreateChatOnceConcurrentProcesses$")
			command.Env = append(os.Environ(), "GATOR_TEST_CREATE_CHAT_ROOT="+root)
			out, err := command.CombinedOutput()
			if err != nil {
				failures <- err
				t.Log(string(out))
				return
			}
			ids <- strings.TrimSpace(string(out))
		}()
	}
	wait.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if first != id {
			t.Fatalf("retry created different chats: %s, %s", first, id)
		}
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	chats, err := store.List(100)
	if err != nil || len(chats) != 1 || chats[0].ID != first || chats[0].HeadRevision != "" || chats[0].SnapshotID != "" {
		t.Fatalf("empty chat: %+v, %v", chats, err)
	}
	if _, err := os.Stat(filepath.Join(store.root, first, "requests.json")); !os.IsNotExist(err) {
		t.Fatalf("created an admission: %v", err)
	}
	if _, err := store.CreateChatOnce(t.TempDir(), "same-client-request-1234", time.Now()); err == nil {
		t.Fatal("same request changed source")
	}
	other, err := store.CreateChatOnce(root, "different-client-request-1234", time.Now())
	if err != nil || other.ID == first {
		t.Fatalf("new request: %+v %v", other, err)
	}
}

func TestCreateChatOnceInterruptedPublicationAndDeletion(t *testing.T) {
	for _, publishing := range []bool{false, true} {
		t.Run(map[bool]string{false: "before publication", true: "before rename"}[publishing], func(t *testing.T) {
			store, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			source, key := t.TempDir(), "recover-creation-request"
			created, err := store.CreateChatOnce(source, key, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256([]byte(key))
			hash := hex.EncodeToString(sum[:])
			path := filepath.Join(store.root, ".creations", hash+".json")
			var receipt chatCreation
			if err := readJSON(path, &receipt); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), source) {
				t.Fatalf("receipt retained the raw workspace path: %v", err)
			}
			receipt.Publishing = publishing
			if err := writeJSON(path, receipt); err != nil {
				t.Fatal(err)
			}
			// Recreate the durable state left immediately before publication.
			if err := os.Rename(filepath.Join(store.root, created.ID), filepath.Join(store.root, ".creations", "stage-"+hash)); err != nil {
				t.Fatal(err)
			}
			retried, err := store.CreateChatOnce(source, key, time.Now())
			if err != nil || retried.ID != created.ID {
				t.Fatalf("interrupted retry: %+v %v", retried, err)
			}
			if err := os.RemoveAll(filepath.Join(store.root, created.ID)); err != nil {
				t.Fatal(err)
			}
			if _, err := store.CreateChatOnce(source, key, time.Now()); err == nil {
				t.Fatal("recreated a deleted chat")
			}
			if chats, err := store.List(100); err != nil || len(chats) != 0 {
				t.Fatalf("deleted chat returned: %+v %v", chats, err)
			}
		})
	}
}

func TestCreateChatOnceRejectsInvalidIdentityAndLinkedStorage(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "short", "../../outside-request", strings.Repeat("a", 97)} {
		if _, err := store.CreateChatOnce(t.TempDir(), key, time.Now()); err == nil {
			t.Fatal("accepted", key)
		}
	}
	if _, err := store.CreateChatOnce("relative", "valid-creation-request", time.Now()); err == nil {
		t.Fatal("accepted relative source")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(store.root, ".creations")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := store.CreateChatOnce(t.TempDir(), "valid-creation-request", time.Now()); err == nil {
		t.Fatal("followed a linked receipt directory")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("changed linked directory: %v %v", entries, err)
	}
}
