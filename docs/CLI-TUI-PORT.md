# CLI/TUI product port

7 November 2026. Product direction: CLI/TUI only, with chat as the default
surface. The prior OpenCode-derived Solid/OpenTUI renderer is restored from
`46be9ba8d^`. Current Go backend code is retained rather than rolled back.
Electron is removed after its newer interactions have terminal entry points and
checks. The requested animated mascot follows that removal.

| Capability | Terminal destination | Authority |
| --- | --- | --- |
| Chat, queues, steering, approvals, independent sessions, recovery | Chat, `/queue`, `/chats` | Current Go journals, approval tokens and host sessions |
| Models, effort, providers, local models and settings | `/model`, `/models`, `/settings` | Existing Go forms and reviews |
| Connections, Gmail send and frozen originals | `/connections`, inline approvals, `/receipts` | Current connector/action code retained |
| Goals, scheduling, updates, memory and knowledge | Chat tools, `/following`, `/saved`, typed pages | Current Go services retained |
| Drafts, attachments, files, Office/PDF/images and export | `/drafts`, `/attach-file`, `/files` | Shared immutable files and exports |
| Usage, lifecycle, backups, retention and service repair | `/usage`, `/data`, `/recovery` | Current Go reviews |
| Source selection, retained sources and code evidence | Port in progress | Current source IDs and verified manifests |
| Reviewed task-plan admission, edits and start | Port in progress | Current revision/digest-bound protocol |
| Branches and individual specialist controls | Port in progress | Current Go lineage and live tokens |
| Local Whisper, dictation and system speech | Port in progress | Current speech configuration/downloads/usage |
| Link/message reuse, copy and read aloud | Port in progress | Private drafts; explicit send required |
| Optional companion and external tmux | Retained independent extension | Existing authentication and grants |
| Distribution and updates | Complete terminal packages | Explicit installer/switch/rollback; profiles retained |

Verification results are recorded as checks complete. Historical migration
results are not evidence for the restored frontend against today's backend.
