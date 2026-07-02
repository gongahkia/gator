# paw — Patch Format (`docs/PATCH_FORMAT.md`)

The `edit` stage (brain) emits a **unified diff**; the `patch` stage (`internal/patch/`) applies
it deterministically. No model touches the filesystem.

---

## 1. Accepted grammar

Standard unified diff, one or more file sections:

```
--- a/relative/path/to/file.go
+++ b/relative/path/to/file.go
@@ -START,COUNT +START,COUNT @@ optional section heading
 context line (leading space)
-removed line
+added line
 context line
```

Rules `edit` MUST follow (enforced by the system prompt in `internal/edit/prompt.go` and checked
by the parser):
- Paths are **relative to `Cwd`**, prefixed `a/` and `b/`. Reject absolute paths and any `..`
  path segments (path-escape guard).
- File creation: `--- /dev/null` then `+++ b/newfile`.
- File deletion: `--- a/oldfile` then `+++ /dev/null`.
- Hunks must have correct `@@` line ranges; context lines must match the current file content.
- No trailing prose, no code fences, no commentary in the diff payload. (`edit` returns the diff
  in the `Patch.UnifiedDiff` field; any surrounding text the model emits is stripped before
  parsing by extracting from the first `--- ` to the last hunk line.)

---

## 2. Application strategy (deterministic)

Use a well-tested Go library rather than hand-rolling. **Pinned choice:**
`github.com/bluekeyes/go-gitdiff/gitdiff` — parses unified diffs (including git extended headers,
creations, deletions, renames) and applies them to file contents.

Apply algorithm in `internal/patch/apply.go`:
1. Parse the diff into file patches (`gitdiff.Parse`).
2. For each file patch:
   - Resolve and validate the target path is inside `Cwd`.
   - Read current file bytes (empty for `/dev/null` source).
   - `gitdiff.Apply(out, srcReader, filePatch)`; on context-mismatch error, do NOT force —
     return a structured apply error naming the file and the first failing hunk.
   - Write result atomically (write temp file in same dir, `os.Rename`).
3. If ANY file patch fails, roll back all writes made in this apply call (keep a backup of each
   touched file's original bytes in memory; restore on failure) and return the error.

---

## 3. Failure handling / brain retry

When apply fails, `edit` receives the structured error and re-prompts the brain **once** with:
the failing file's current content (a fresh `sed`-style slice around the intended hunk) plus the
apply error message, asking for a corrected diff. Bounded to 1 retry per step to protect the
token budget; after that the step is marked failed and the loop's `plan` stage decides how to
proceed on the next turn.

---

## 4. Fuzz-friendliness (tested)

`internal/patch/apply_test.go` includes: create-new-file, delete-file, single-hunk edit,
multi-hunk edit, multi-file diff, context-mismatch rejection, and path-escape rejection
(`../../etc/passwd` must be refused). Fixtures live in `testdata/patches/`.
