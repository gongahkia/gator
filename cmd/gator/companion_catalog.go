package main

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/worksession"
)

// This read-only transport uses the same bounded catalog as the local UI. It
// never starts a conversation, runs retention maintenance or loads a provider.
// The catalog may update its disposable local index, under the profile guard.
func companionCatalog(query, scope, after string, out io.Writer) error {
	if err := preparePlatformProfile(); err != nil {
		return err
	}
	root, err := state.ResolveDir("")
	if err != nil {
		return err
	}
	release, err := state.Guard(root, false)
	if err != nil {
		return err
	}
	defer release()
	store, err := worksession.Open(root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	page, err := store.PageConversationsContext(ctx, worksession.ConversationQuery{Text: query, Scope: scope, After: after, Limit: 50})
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(page)
}
