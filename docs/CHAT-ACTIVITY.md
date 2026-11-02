# Observed chat activity

Electron's existing ordered activity trace now includes a finite projection of actual native tool results:

- Search-result domains and fetched-source domains. A search result is not labelled as a page read. Fetched evidence can open the retained source after the run is saved.
- Relative source/code reads and paths reported by Git after a successful staged patch. A narration mentioning a file does not mark it edited.
- Recorded program exit codes and timeouts. Exit 0 is not a claim that the user's task succeeded.
- Verified/failed candidate checks, with the distinction that application review is still required.
- A waiting state from an actual command-approval request. Cancelling while waiting leaves that operation unfinished, not successful.

Live activity remains owned by the existing Work runner. There is no additional execution process, polling loop, model request, terminal input path or persistence store. The optional activity sidecar remains separate from canonical replay; old sidecars work, and malformed/newer/unsafe display details cannot prevent restoring the answer.

Changed-file actions are available only when the current conversation contains a retained candidate for the exact run and path. They open the existing verified diff inspector. If more than one candidate matches, the Files view is filtered to that run/path for explicit selection. Unretained staged edits remain plain metadata. Source actions similarly use the retained-source inspector, never the current working file.

## Bounds and privacy

The existing limits of 160 entries and 64 KiB of public narration remain. Details are limited to 12 per tool and 32 KiB per trace. Tool results larger than 128 KiB are not parsed for display details. Git path projection has a two-second deadline, a bounded runner output and at most 32 paths. Shortening is explicit. Optional path-projection failure after applying a patch does not turn an already applied change into a retryable error.

Details have no raw argv, tool arguments, output, diagnostics, complete URLs or provider payloads. Domains exclude URL paths, queries and credentials; paths must be bounded relative paths. Unknown tools are not interpreted as these native contracts. Public narration is already-visible assistant text, not hidden reasoning. It can still contain user-provided information; normal conversation retention rules apply.

An observed command result or frozen candidate does not waive command approvals, candidate verification, evidence sealing, artifact review or external-terminal grants. Native coding controls remain separate from terminal input.

## Interaction review

Scope: React/Lucide and existing CSS tokens, activity groups and their bounded inspectors. Full review of the changed surface only.

| Category | Evidence | Result |
| --- | --- | --- |
| Typography | Domain/path wrapping, monospace relative paths, explicit result labels | Existing font and size tokens retained |
| Surfaces | Group disclosure, compact metadata, file/source inspector actions | Existing surfaces; interactive detail chips use 40 px minimum height |
| Animations | Existing disclosure interruption, inert closed content and reduced-motion journey | Existing animation retained, no animation added to frequent tool updates |
| Icons | Existing Lucide set; waiting clock and outcome indicators | Labels accompany state icons |
| Performance | Metadata/count bounds; no backend reads until an inspector action | Finite projection, no new polling |

Considered and rejected: automatic live-file previews (would substitute changing workspace bytes for retained evidence); full command/output chips (privacy and response-size cost); independent tool-result storage (duplicates existing replay and optional display sidecar).

## Verification

Free fixtures use real Electron, Go Work tools and disposable projects; their model responses are scripted. Display-only ordering/domain fixtures do not claim live web acceptance.

- `go test ./internal/workactivity ./internal/worksession ./internal/tools`: PASS. An earlier attempt hit `TestSQLiteCatalogConcurrentReadersAndOrganization`; repeated race testing exposed a real pre-existing metadata-read race, corrected and documented in [Conversation index consistency](CHAT-CATALOG-CONSISTENCY.md).
- `go test -race ./internal/workactivity ./internal/worksession ./internal/tools`: PASS (activity cached, session 26.497 s, tools 14.241 s) after the metadata-lock correction.
- `npm run check --prefix desktop` and `npm run build --prefix desktop`: PASS. Existing CSS `::highlight` bundler warnings remain.
- Electron implementation/repair, internal worker and reviewer, cumulative follow-up, inspect-only policy, and cancellation while approval is pending: 7 PASS. The separately gated paid coding evaluation was skipped. Changed-path actions opened the sealed diff; failed and successful program exits were visible.
- Activity ordering/disclosure/scroll/reduced-motion and restart/source-inspector journeys: 2 PASS in 10.9 s on the focused rerun. The initial restart assertion selected a nonexistent `region` role for the sidebar; its locator was corrected to the retained-content label.

`npm test --prefix desktop`: desktop 30 PASS / 1 SKIP; application host 75 PASS / 1 SKIP.

Native microphone, physical phone, signed installer and live-provider acceptance are not established by these fixtures. No paid API evaluations were run.
