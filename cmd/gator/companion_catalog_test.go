package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/worksession"
)

func TestCompanionCatalogPagesRetainedChatsWithoutStartingWork(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_INTERNAL_COMMANDS", "")
	t.Setenv("GATOR_COMPANION_CATALOG", "")
	args := []string{"companion-catalog", "", "active", ""}
	if err := runDesktopRuntime(args, io.Discard); err == nil {
		t.Fatal("catalog accepted without host routing")
	}
	t.Setenv("GATOR_COMPANION_CATALOG", "1")
	store, err := worksession.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 53; i++ {
		if _, err := store.Create(fmt.Sprintf("Catalog chat %02d", i), root, "snap-fixture", time.Now().Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	read := func(args []string) worksession.ConversationPage {
		t.Helper()
		var out bytes.Buffer
		if err := runDesktopRuntime(args, &out); err != nil {
			t.Fatal(err)
		}
		var page worksession.ConversationPage
		if err := json.Unmarshal(out.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	first := read(args)
	if len(first.Items) != 50 || first.Next == "" {
		t.Fatalf("first page: %+v", first)
	}
	args[3] = first.Next
	second := read(args)
	if len(second.Items) != 3 || second.Next != "" {
		t.Fatalf("second page: %+v", second)
	}
	seen := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		if seen[item.ID] {
			t.Fatal("duplicate page item", item.ID)
		}
		seen[item.ID] = true
	}
	args[1], args[3] = "Catalog chat 52", ""
	if page := read(args); len(page.Items) != 1 || page.Items[0].Title != args[1] {
		t.Fatalf("search: %+v", page)
	}
	args[3] = first.Next
	if err := runDesktopRuntime(args, io.Discard); err == nil {
		t.Fatal("accepted a cursor from a different search")
	}
	if items, err := store.List(100); err != nil || len(items) != 53 {
		t.Fatalf("catalog created a conversation: %d, %v", len(items), err)
	}
}
