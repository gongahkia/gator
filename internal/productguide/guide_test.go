package productguide_test

import (
	"slices"
	"testing"

	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/productguide"
)

// A provider surface change must reconcile its public capability guide. This
// compares the independently maintained descriptions to production registration,
// so adding/removing a built-in cannot silently leave the guide stale.
func TestGoogleCapabilityInventoryMatchesRegisteredOperations(t *testing.T) {
	guide := productguide.Current()
	described := slices.Clone(guide.GoogleOperations)
	var registered []string
	for _, op := range (connector.Descriptor{Kind: connector.KindGoogle, Authentication: connector.AuthNone}).Operations() {
		registered = append(registered, op.ID)
	}
	slices.Sort(described)
	slices.Sort(registered)
	if !slices.Equal(described, registered) {
		t.Fatalf("reconcile shared product guide with Google tools: described=%v registered=%v", described, registered)
	}
}
