package retrieval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type countingEmbedder struct{ inputs int }

func (*countingEmbedder) ID() string { return "test/one" }
func (e *countingEmbedder) Embed(_ context.Context, input []string) ([][]float32, error) {
	e.inputs += len(input)
	result := make([][]float32, len(input))
	for i, value := range input {
		vector := make([]float32, 16)
		if len(value) > 0 && value[0] == 'o' {
			vector[0] = 1
		} else {
			vector[1] = 1
		}
		result[i] = vector
	}
	return result, nil
}

func TestIndexIncrementalUpdateAndForget(t *testing.T) {
	state := t.TempDir()
	embedder := &countingEmbedder{}
	docs := []Document{{ID: "first", Text: "olive preference"}, {ID: "second", Text: "tea preference"}}
	search := func(documents []Document) []Hit {
		t.Helper()
		hits, err := Search(context.Background(), state, "memory/global", "olive", documents, embedder, 5)
		if err != nil {
			t.Fatal(err)
		}
		return hits
	}
	if hits := search(docs); len(hits) != 1 || hits[0].ID != "first" {
		t.Fatalf("initial hits: %#v", hits)
	}
	if embedder.inputs != 3 {
		t.Fatalf("first search embedded %d inputs, want 3", embedder.inputs)
	}
	search(docs)
	if embedder.inputs != 4 {
		t.Fatalf("unchanged documents were reembedded: %d", embedder.inputs)
	}
	docs[0].Text = "tea preference"
	if hits := search(docs); len(hits) != 0 {
		t.Fatalf("corrected document retained stale vector: %#v", hits)
	}
	if embedder.inputs != 6 {
		t.Fatalf("correction embedded %d inputs, want 6", embedder.inputs)
	}
	docs[0].Text = "olive preference"
	search(docs)
	if err := DropID(state, "first"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state, "gator", "retrieval", indexName(embedder.ID(), "memory/global"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var index indexFile
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if _, exists := index.Entries["first#0"]; exists {
		t.Fatal("forgotten vector remains on disk")
	}
	if hits := search(docs); len(hits) != 0 {
		t.Fatalf("a stale caller reindexed a forgotten document: %#v", hits)
	}
	if hits := search(docs[1:]); len(hits) != 0 {
		t.Fatalf("removed document remains searchable: %#v", hits)
	}
}
