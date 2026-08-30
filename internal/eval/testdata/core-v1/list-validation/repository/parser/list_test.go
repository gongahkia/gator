package parser

import (
	"reflect"
	"testing"
)

func TestParseListTrimsValidItems(t *testing.T) {
	got, err := ParseList(" alpha, beta ")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseList() = %#v, want %#v", got, want)
	}
}
