# Personal knowledge and skills

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

## Notes

```sh
gator note add --title 'Venue decision' --goal GOAL_ID --source-ref 'source/venue-email.md:12' --body 'Venue B is preferred; verify capacity by Friday.'
gator note add --title 'Meeting notes' --file ./meeting.md
gator note search --goal GOAL_ID 'venue capacity'
gator note show NOTE_ID
gator note edit NOTE_ID --file ./revised-meeting.md
gator note edit NOTE_ID --source-ref 'source/revised-venue.md:4'
gator note edit NOTE_ID --clear-source-refs
gator note path NOTE_ID
gator note remove NOTE_ID --yes
```

A note is either global or attached to one goal. Goal-scoped notes are never
returned to another goal's Work tools. Search is lexical and bounded. It indexes
source reference terms so a note can be found by a document name, but it does
not claim semantic recall. Source references are user-maintained provenance
labels, not proof that the source still says the same thing; check the current
source before relying on them. The agent may try up to three alternate terms
when its first knowledge search misses, and each result shows which query
matched. This remains lexical search and can miss a different paraphrase or
return a weak match. Short `gator memory` records retain their separate
expiry and correction controls. `knowledge_search` and `knowledge_read` let the
main agent retrieve both notes and active memories during Work. They accept IDs
and query text, never arbitrary filesystem paths.

In the TUI, `/note add TITLE | BODY`, `/note list`, `/note search QUERY`, and
`/note edit ID | BODY` use the current goal by default. The terminal CLI supports
larger files and full metadata options.

## Skills

```sh
gator skill create daily-brief --description 'Use for morning briefings' --file ./SKILL.md
gator skill list
gator skill show daily-brief
gator skill edit daily-brief --description 'Use for daily briefings' --file ./revised-SKILL.md
gator skill disable daily-brief
gator skill enable daily-brief
gator skill remove daily-brief --yes
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
gator suggestion list
gator suggestion show SUGGESTION_ID
gator suggestion accept SUGGESTION_ID
gator suggestion reject SUGGESTION_ID
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
