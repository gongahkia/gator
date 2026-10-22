# Personal knowledge and skills

**Interface status:** Gator is a desktop app. Shell examples below are source-checkout developer operations with an explicit internal opt-in; see [development instructions](DEVELOPMENT.md). They do not install or enable a public CLI.

Gator keeps short facts and decisions in `gator memory`. Longer Markdown notes
and reusable procedures live in a private filesystem knowledge base under the
resolved state directory:

```text
gator/knowledge/
  notes/note-<id>/meta.json
  notes/note-<id>/content.md
  skills/<name>/meta.json
  skills/<name>/SKILL.md
```

Directories are private (`0700`) and files are private (`0600`). Gator accepts
only bounded UTF-8 text, exact generated note IDs or validated skill names, and
regular files. Symlinks and path traversal are rejected at the store boundary.
The Markdown is inspectable and may be edited directly; invalid or oversized
edits fail closed the next time Gator loads that entry. No skill content is
executed as a shell command or loaded as a plugin.

For a short memory supported by a local file, use `gator memory add
--source-file PATH TEXT`. Gator stores the path and a fingerprint of the file
(up to 4 MiB). If it changes or disappears, `memory show` reports the source
status and automatic recall skips that memory. After reviewing the current
file, use `gator memory correct ID --source-file PATH TEXT` to update both the
fact and its fingerprint. The fingerprint detects file changes; it does not
prove that the file supports the memory's claim. Plain `--source REF` remains
an unverified label.

`gator memory search QUERY` shows lexical terms that matched. At the start of
Work, Gator combines lexical and semantic recall over eligible memories and
notes. The default semantic provider is local Ollama `embeddinggemma:latest`.
Install it explicitly with `gator memory embeddings install --yes` (about
622 MB), and check readiness with `gator memory embeddings status`. If Ollama
or the model is unavailable, Work continues with lexical recall. Goal scope,
expiry, and changed source fingerprints are checked before memory indexing.
Retrieval reasons are retained with each Work decision, but they do not prove a
fact is true. Current sources still take precedence when they disagree.

Cloud embeddings are optional and separate from the chat model selection:
`gator memory embeddings set openai --allow-upload` or `gator memory embeddings
set gemini --allow-upload`. These settings permit Gator to upload eligible
saved memory and note text and the current query to that provider for
embedding. Use `gator memory embeddings set ollama` to return to local recall.
The private vector index contains embeddings and content hashes rather than
plain text. It updates changed records incrementally and removes forgotten
records. Saved source text and instructions never grant tool authority.

## Memory and privacy in the desktop

Settings → **Memory & privacy** separates two preferences:

- **Learn from chats** controls automatic saves of enduring facts and preferences. Turning it off keeps existing memories available and still permits an explicit request to remember or forget.
- **Refer to earlier chats** controls the earlier-conversation search/read tools on new requests. Turning it off retains this conversation and selected saved memories. Requests already running keep the context and tools with which they started.

Neither switch deletes stored messages or trains the model. **Saved** manages saved knowledge; **Data & backups** manages retained data. Forgetting a saved memory blocks silently relearning its canonical subject, but prior messages, files and retained run evidence may still contain the words. A new chat starts a fresh working context within the same local profile; devices do not sync automatically.

A collapsed **Saved to memory** receipt beneath an answer opens the current saved fact and its scope. Edit it there, undo an unchanged new save, or review forgetting an edited fact. The Saved screen uses the same version checks: another chat's correction cannot be silently overwritten. If a memory changes or disappears while you edit, your draft stays open until you load the current version or cancel. Historical messages and context already supplied to running work are kept.

## Notes

In the desktop, Memory & privacy (or `/memory` in command search) opens the readiness and setup panel. It explains
three separate stages: private local storage, retrieval through basic matching
or the selected embedding provider, and chat processing through `/model`.
Local retrieval does not prevent selected memories from being sent to a cloud
chat model. Local-model installation runs with progress and cancellation; cloud
retrieval requires a separate review of the saved-text upload.

`/context` opens context and sources for the latest Work result; `/context ID`
opens a retained run. Expand a saved item with Enter to inspect its excerpt,
scope, source labels, and revision. `E` corrects the canonical saved text, `F`
reviews forgetting, and `O` toggles omission from automatic recall and knowledge
tools for the next run. Historical excerpts describe what was supplied at run
start; they are not rewritten by a later correction or forgetting action.
Existing conversation text is not erased by omission. A changed file-linked
source still needs review and relinking before automatic recall can use it.

These excerpts and source references describe inputs, not hidden model
reasoning or proof that a source supports an answer's claim. Feedback ratings
are retained for review and do not automatically retrain the selected model.

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note add --title 'Venue decision' --goal GOAL_ID --source-ref 'source/venue-email.md:12' --body 'Venue B is preferred; verify capacity by Friday.'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note add --title 'Meeting notes' --file ./meeting.md
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note search --goal GOAL_ID 'venue capacity'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note show NOTE_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note edit NOTE_ID --file ./revised-meeting.md
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note edit NOTE_ID --source-ref 'source/revised-venue.md:4'
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note edit NOTE_ID --clear-source-refs
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note path NOTE_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator note remove NOTE_ID --yes
```

A note is either global or attached to one goal. Goal-scoped notes are never
returned to another goal's Work tools. The explicit search command is lexical
and bounded. It indexes
source reference terms so a note can be found by a document name, but it does
not claim a semantic match. Source references are user-maintained provenance
labels, not proof that the source still says the same thing; check the current
source before relying on them. The agent may try up to three alternate terms
when its first knowledge search misses, and each result shows which query
matched. This remains lexical search and can miss a different paraphrase or
return a weak match. Short `gator memory` records retain their separate
expiry and correction controls. `knowledge_search` and `knowledge_read` let the
main agent retrieve both notes and active memories during Work. They accept IDs
and query text, never arbitrary filesystem paths.

At the start of Work, Gator also puts up to four matching note leads in the
model context. Each lead has its note ID, goal/global scope, retrieval reason,
up to two source labels, and a short excerpt. The full note
stays in the private store and is available through `knowledge_read`. A lead
is a retrieval hint, not a verified fact: source labels can be stale, and a
match can be irrelevant. Corrected or removed notes are reflected in the next
Work run, and notes from another goal never enter the selection. Work uses the
current request and ongoing goal context to rank eligible notes.

In the TUI, `/note add TITLE | BODY`, `/note list`, `/note search QUERY`, and
`/note edit ID | BODY` use the current goal by default. The terminal CLI supports
larger files and full metadata options.

## Skills

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator skill create daily-brief --description 'Use for morning briefings' --file ./SKILL.md
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator skill list
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator skill show daily-brief
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator skill edit daily-brief --description 'Use for daily briefings' --file ./revised-SKILL.md
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator skill disable daily-brief
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator skill enable daily-brief
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator skill remove daily-brief --yes
```

In the TUI, `/skill create NAME | WHEN_TO_USE | BODY` and
`/skill edit NAME | WHEN_TO_USE | BODY` save a procedure directly. During Work,
the model can call `skill_list` to see enabled descriptions and `skill_read` to
load one full procedure. Skills are not automatically copied into every prompt.
They cannot create tools, grant connected access, widen the process sandbox,
or authorize an external action. A user creates or edits them explicitly; the
model currently cannot silently persist a skill or a note.

## Goal reviewer

For an active goal, the main agent can delegate a read-only `goal_reviewer`.
Its snapshot includes the current goal, up to five revisions from each relevant
conversation branch, up to eight goal-linked follow-up results, and bounded
references to saved notes and memories. It can inspect the run's frozen source
and use goal-scoped knowledge search/read. It has no write, command, connector,
browser, scheduling, or recursive delegation tool. Its recommendation returns
to the main agent, which remains responsible for checking evidence and deciding
what to tell or ask the user.

Stored notes, skill text, source files, and prior model responses are reference
data. Their contents cannot change Gator's host-owned authority. The existing
Code specialist still uses its strict process sandbox for any command execution;
the knowledge tools themselves do not start a process.

## Review agent suggestions

During Work, Gator may queue a candidate ongoing goal, note, skill, goal-plan edit, or
goal-linked recurring check. A goal proposal includes a starting plan and may include an inspect-only recurring check. The
candidate records its source Work ID and is not used as knowledge until you
accept it. Review from the terminal UI with `/suggestions` or from the shell:

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator suggestion list
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator suggestion show SUGGESTION_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator suggestion accept SUGGESTION_ID
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator suggestion reject SUGGESTION_ID
```

`gator suggestion show` includes `reference_integrity` for note source references
and proposed plan evidence. It checks them against the retained, verified Work
bundle and current local source files. `verified` means the reference points to
observed material; it does not mean the material supports the suggested claim.
`unverified` or `work_unavailable` calls for manual review before acceptance.

A plan suggestion carries the exact plan revision it was based on. If you
changed the plan since then, acceptance fails and leaves the suggestion pending
for review or rejection.

A schedule suggestion shows its five-field cron expression, IANA timezone,
task, and optional local change gate before acceptance. Accepting it creates an
inspect-only goal job; it cannot grant unattended sending or publishing.

## Recovery, usage, and data controls

[Recovery, shared usage, and private data](RECOVERY_USAGE_DATA.md) documents
/recovery, /usage, /data, their CLI equivalents, conservative defaults, and
verification limits.

[Everyday overview, restore, and migration](OVERVIEW_RESTORE.md) describes the
four-question home view, exact actions, fresh-profile recovery, and reviewed
source-folder relinking.
