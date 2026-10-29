package retrieval_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/retrieval"
)

type preferenceEmbedder struct{}

func (preferenceEmbedder) ID() string { return "test/preferences" }
func (preferenceEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	var vectors [][]float32
	for _, text := range texts {
		vector := make([]float32, 16)
		if strings.Contains(text, "olive") {
			vector[0] = 1
		} else {
			vector[1] = 1
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func assertRecall(t *testing.T, state, query string, doc retrieval.Document, want bool) {
	t.Helper()
	hits, err := retrieval.Search(context.Background(), state, "personal", query, []retrieval.Document{doc}, preferenceEmbedder{}, 4)
	if err != nil || (len(hits) == 1 && hits[0].ID == doc.ID) != want {
		t.Fatalf("recall %q: hits=%v, err=%v, want match=%t", query, hits, err, want)
	}
}

func TestChatMemorySemanticLifecycle(t *testing.T) {
	for _, correction := range []string{"chat", "receipt", "reviewed", "cli", "source-file"} {
		t.Run(correction, func(t *testing.T) {
			state := t.TempDir()
			store, err := memory.Open(state)
			if err != nil {
				t.Fatal(err)
			}
			input := memory.ChatInput{Subject: "preference.food", Kind: "preference", Quote: "I prefer olive dishes.", ConversationID: "work-one", RunID: "run-one"}
			record, err := store.RememberChat(input)
			if err != nil {
				t.Fatal(err)
			}
			assertRecall(t, state, "olive", retrieval.Document{ID: record.ID, Text: record.Text}, true)
			// Reproduce caches left by the old save/correction path. An explicit
			// correction of a live record must repair those without reviving a
			// subsequently forgotten record from a stale search snapshot.
			if err := retrieval.DropID(state, record.ID); err != nil {
				t.Fatal(err)
			}
			const text = "I prefer tea."
			switch correction {
			case "chat":
				input.Quote, input.ReplaceID, input.ExpectedUpdated = text, record.ID, record.UpdatedAt.Format(time.RFC3339Nano)
				_, err = store.RememberChat(input)
			case "receipt":
				err = store.ReviewChat(record.ID, input.ConversationID, "", record.UpdatedAt.Format(time.RFC3339Nano), "correct", text)
			case "reviewed":
				_, err = store.CorrectReviewed(record.ID, text, "user", record.UpdatedAt.Format(time.RFC3339Nano))
			case "cli":
				_, err = store.Correct(record.ID, text, "user")
			case "source-file":
				path := filepath.Join(t.TempDir(), "preference.txt")
				if err := os.WriteFile(path, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
				_, err = store.CorrectFromFile(record.ID, text, path)
			}
			if err != nil {
				t.Fatal(err)
			}
			record, err = store.Load(record.ID)
			if err != nil {
				t.Fatal(err)
			}
			doc := retrieval.Document{ID: record.ID, Text: record.Text}
			assertRecall(t, state, "olive", doc, false)
			assertRecall(t, state, "tea", doc, true)
			if err := store.Forget(record.ID); err != nil {
				t.Fatal(err)
			}
			assertRecall(t, state, "tea", doc, false)
		})
	}
}

func TestReviewedNoteSemanticLifecycle(t *testing.T) {
	state := t.TempDir()
	store, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	note, err := store.AddNote("Food", "olive dishes", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRecall(t, state, "olive", retrieval.Document{ID: note.ID, Text: note.Body}, true)
	note, err = store.ReviewNote(note.ID, "", note.Revision(), "correct", "tea")
	if err != nil {
		t.Fatal(err)
	}
	doc := retrieval.Document{ID: note.ID, Text: note.Body}
	assertRecall(t, state, "olive", doc, false)
	assertRecall(t, state, "tea", doc, true)
	if _, err := store.ReviewNote(note.ID, "", note.Revision(), "forget", ""); err != nil {
		t.Fatal(err)
	}
	assertRecall(t, state, "tea", doc, false)
}
