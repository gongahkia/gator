I've learned a lot by looking at other people's code so
maybe someone can learn something from looking at mine.

= = = = = = = =

Gator is a terminal-native personal agent for ongoing goals.

Gator lets you keep an ongoing goal and continue its conversation across
terminal sessions. The terminal home shows active goals and unread follow-up
results; /goals resumes one. You can save facts, preferences, decisions, and
notes with gator memory (or /memory), then search, correct, expire, or forget
them. Relevant active memories are retrieved for each Work turn alongside the
selected goal's current context. Gator can answer questions, research selected
sources, analyze data, create documents, and prepare actions through selected
tools. A goal can have a recurring check with gator goal schedule. The local
job supervisor runs scheduled checks and sends results to Gator's inbox; on
macOS and Linux, opt-in per-user services can keep it running after the CLI
closes. External changes still require explicit approval. See docs/PERSONAL_AGENT_DIRECTION.md
for the product scope and current limits.

Longer notes and reusable text skills can be saved with `gator note` and
`gator skill` (or `/note` and `/skill` in the terminal UI). They live in a
private, inspectable knowledge directory and are loaded only through bounded
read tools. An active goal can use a read-only goal reviewer to compare recent
work and follow-ups before recommending a next step. See docs/KNOWLEDGE.md.

Gator currently pins Go 1.25.13.

Build with `make build`, then run `./bin/gator` for the interactive terminal.
The green React terminal interface uses Node.js 22 or newer; the same Go
binary falls back to its native terminal interface if Node.js is unavailable.
Set `GATOR_NATIVE_TUI=1` to choose the native interface explicitly. Both
interfaces use the same Go Work engine, conversations, commands, and approvals.

= = = = = = = =

🐊
