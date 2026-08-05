package store

import "testing"

func TestEventPageCursorIsOpaqueAndRoundTrips(t *testing.T) {
	cursor := encodeEventPageCursor(42)
	if cursor == "42" {
		t.Fatal("event cursor exposes the database sequence")
	}
	id, err := decodeEventPageCursor(cursor)
	if err != nil || id != 42 {
		t.Fatalf("cursor=%q id=%d err=%v", cursor, id, err)
	}
	if _, err := decodeEventPageCursor("42"); err == nil {
		t.Fatal("expected raw event id cursor rejection")
	}
}
