package catalog

import "testing"

func TestApplyChangesExplicitName(t *testing.T) {
	name := "renamed"
	got, err := Apply([]Entry{{ID: "a", Name: "old", Enabled: true}}, []Patch{{ID: "a", Name: &name}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "renamed" || !got[0].Enabled {
		t.Fatalf("Apply() = %#v", got)
	}
}
