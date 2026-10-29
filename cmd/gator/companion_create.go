package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/worksession"
)

func companionCreateChat(requestID string, out io.Writer) error {
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
	source, err := os.Getwd()
	if err != nil {
		return err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	store, err := worksession.Open(root)
	if err != nil {
		return err
	}
	chat, err := store.CreateChatOnce(source, requestID, time.Now())
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		RequestID      string `json:"request_id"`
		ConversationID string `json:"conversation_id"`
		Source         string `json:"source"`
	}{requestID, chat.ID, chat.SourcePath})
}
