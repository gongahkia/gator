package run

import (
	"context"
	"strings"
	"testing"
)

func TestWriterPatchComparisonUsesThreeWayMergeWithoutMovingHEAD(t *testing.T) {
	repository := featureRepository(t)
	ctx := context.Background()
	head, err := writerGitOutput(ctx, repository, nil, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	head = strings.TrimSpace(head)
	alpha, err := createWriterPatchCommit(ctx, repository, head, []byte(newFilePatch("alpha.txt", "alpha\n")))
	if err != nil {
		t.Fatalf("create alpha comparison commit: %v", err)
	}
	beta, err := createWriterPatchCommit(ctx, repository, head, []byte(newFilePatch("beta.txt", "beta\n")))
	if err != nil {
		t.Fatalf("create beta comparison commit: %v", err)
	}
	status, tree, detail := compareWriterPatchCommits(ctx, repository, []delegatedWriterReport{{PatchCommit: alpha}, {PatchCommit: beta}})
	if status != "clean" || tree == "" || !strings.Contains(detail, "semantic review") {
		t.Fatalf("clean comparison = status %q tree %q detail %q", status, tree, detail)
	}
	after, err := writerGitOutput(ctx, repository, nil, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(after) != head {
		t.Fatalf("comparison moved HEAD from %s to %s", head, strings.TrimSpace(after))
	}

	first, err := createWriterPatchCommit(ctx, repository, head, []byte(newFilePatch("shared.txt", "one\n")))
	if err != nil {
		t.Fatal(err)
	}
	second, err := createWriterPatchCommit(ctx, repository, head, []byte(newFilePatch("shared.txt", "two\n")))
	if err != nil {
		t.Fatal(err)
	}
	status, tree, _ = compareWriterPatchCommits(ctx, repository, []delegatedWriterReport{{PatchCommit: first}, {PatchCommit: second}})
	if status != "conflict" || tree != "" {
		t.Fatalf("conflicting comparison = status %q tree %q", status, tree)
	}
}

func newFilePatch(name, contents string) string {
	return "diff --git a/" + name + " b/" + name + "\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/" + name + "\n" +
		"@@ -0,0 +1 @@\n" +
		"+" + strings.TrimSuffix(contents, "\n") + "\n"
}
