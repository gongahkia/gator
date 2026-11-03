# Configurable web search

Gator supports host-selected TinyFish, Exa, Tavily and Brave search adapters.
They reuse the existing bounded HTTP tools, public-address DNS checks, pinned
connections, proxy-free transport, request limits and query approvals. No SDK,
browser service or new application runtime is installed.

## Configure

Set the selected provider and its key in the environment of the Gator process:

| Provider value | Credential variable | API used |
| --- | --- | --- |
| `tinyfish` | `TINYFISH_API_KEY` | `GET https://api.search.tinyfish.ai` |
| `exa` | `EXA_API_KEY` | `POST https://api.exa.ai/search` |
| `tavily` | `TAVILY_API_KEY` | `POST https://api.tavily.com/search` |
| `brave` | `BRAVE_SEARCH_API_KEY` | Existing Brave Web Search endpoint |

For example, the local evaluation's Git-ignored `.env` can contain:

```dotenv
GATOR_WEB_SEARCH_PROVIDER=tinyfish
TINYFISH_API_KEY=your_key_here
```

The evaluation launcher reads those named values and passes them to its child
process. Ordinary Gator reads process environment; it does **not** automatically
source a project's `.env`. Launch/restart the app with the intended environment.
Do not put credentials in project instructions, model arguments, approval text
or committed files.

If exactly one search key is configured and no provider is specified, that
provider is selected. Existing Brave-only configurations retain their behavior.
Multiple keys require an explicit `GATOR_WEB_SEARCH_PROVIDER`. Unknown providers,
missing selected keys and ambiguous selection fail closed with a configuration
error. An HTTP failure does not cause fallback to another vendor, credential or
paid search mode. Existing host diagnostics report the selected provider, not keys.

## Scope, results and cost

The model-facing tool still accepts only a query and result count. Models cannot
choose a provider, API endpoint, key, browser action, agent or research mode.
Queries are sent to the explicitly configured search service.

Native coding tools retain their existing network and query-approval boundaries.
In desktop Work, ask Gator to research the public web. `web_access` checks the
current task's scope/usage, requests additional scope, or revokes it. A request
opens the native **Review web research access** card with the requested sites or
public-web scope, purpose, host-selected service, shared limits and data-sharing
terms. Approving it permits credential-free public HTTPS reads for that task,
including its connected researchers. A public grant can discover previously
unknown sites; a sites grant expands only the named origins. The grant never
selects an account, allows browser login, approves external writes, or applies
to another task. Background checks cannot request an interactive expansion.

Before expansion, Work uses its explicitly selected HTTPS origins. Its search
wrapper adds site operators and independently filters returned URLs by exact
selected scheme/host/port; operators are relevance hints, not authorization.
After a public grant, queries need no site prefix and safe public leads can come
from new sites. Credential URLs, literal private/reserved IPs, insecure schemes,
local names and non-443 ports are omitted. Search snippets are untrusted discovery
leads and do not become factual evidence. `http_fetch` retains an actually read
page before its contents can be cited as checked evidence. There is no fetch or
browser/agent fallback to TinyFish or another vendor. Missing search configuration
still permits reviewed page reads; the status/card reports that search is unavailable.

Scope requests require the live foreground owner. Denied scopes cannot be
requested again within that task, and at most four scope reviews can be opened.
A narrower site request can follow a public-scope denial. Revocation prevents
new admissions without resetting capacity; an already admitted request may finish.
Revocation/cancellation while a review is pending prevents its late approval from
reinstating access. Plan-first execution rechecks the same live reviewed plan
before and after consent. Its additional read grant is recorded as a separate
interaction; it does not change the plan's write, coding or connected-action scope.

Work returns at most 8 KiB of readable page preview to the model. HTML previews
omit script/style/navigation/footer text and normalize visible text; they do not
execute scripts. Exact fetched bytes and their digest remain in the evidence
catalog. `read_evidence` provides bounded raw-byte pages using `offset_bytes`,
`limit_bytes` (at most 32 KiB) and `next_offset_bytes`; UTF-8 boundaries are checked.
`search_evidence` finds an exact case-sensitive literal in retained bytes without
another network request. It returns first/last matching contexts, total/omitted
counts and exact UTF-8-safe offsets, so large HTML does not require scanning
every page into the model context. Defaults are four matches and 400 context
bytes on each side; hard limits are eight matches, a 512-byte query and 1,024
context bytes per side. Only captured bytes are searched, so an upstream page
cut at the fetch limit may need a more focused source. Both evidence reading
paths verify the retained hash. Content-addressed captures are published
atomically so parallel researchers cannot observe another writer's partial file.
Truncation and preview normalization are explicit. Exact quote checks still use
the original bytes, so a normalized preview is not evidence of a literal match
across HTML tags. No full-page completeness or JavaScript rendering is claimed.

All adapters normalize bounded title/URL/snippet/date metadata. At most ten
results are returned, fields and response bodies are bounded. Work allows at most
**eight searches within 32 total admitted web attempts**, shared across manager
and workers. Failed attempts can consume capacity, and a new worker or grant
does not reset it. Native coding HTTP tools keep their eight-request default;
only the host can change their bounded limit. Production search uses the existing
HTTP deadline and public-address connection checks. Fixed provider endpoints
receive keys through headers only. Raw provider error bodies and transport
errors are not exposed; errors retain HTTP status where available.

Public scope preserves private/reserved-address rejection for IPv4/IPv6,
mixed DNS responses, DNS-pinned connections, proxy-free transport and refusal to
follow redirects. These protections were reviewed against the
[OWASP SSRF guidance](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html).
The address review also rejects special-purpose/tunnel destinations that a basic
global-unicast check admits, including private IPv4 hidden in transition forms;
see the [IANA IPv6 registry](https://www.iana.org/assignments/iana-ipv6-special-registry).
Well-known `/96` DNS64 destinations validate the embedded IPv4 address, preserving
public translation while rejecting private/loopback/metadata targets. Local-use
translation prefixes, deprecated tunnels and protocol-only ranges are excluded;
ordinary public IPv4/IPv6 and public IPv4-mapped addresses remain supported.
This is not a complete guarantee against indirect disclosure: queries and URL
paths chosen by the model leave the computer. The review says so and model
guidance prohibits sharing private source content without user intent, but the
host does not semantically inspect outbound text for every possible secret.
Do not interpret network isolation as a prompt-injection or DLP certification.

The interaction journal retains the exact scope review and decision. Work
manifests retain historical scope, service and admitted request/search counts;
those fields do not authorize another run. They do not infer an invoice from a
count. Search charges, free allowances and model fees remain provider-specific.

Tavily explicitly requests `basic` depth, disables automatic parameter selection,
answers, raw content and images. Exa uses standard `auto` search and requests no
extra content/answer/research endpoint. TinyFish uses Search only, never its paid
Agent, Browser or Monitor APIs. Provider pricing, free allowances and account
limits remain provider-specific; changing search provider does not change the
configured language model or its API charges.

Checked again on 3 November 2026: TinyFish's pricing page describes free Search, while its API documentation also
describes a daily allowance and possible wallet behavior beyond it. Do not infer
unlimited free service from the marketing page. A 402/429 is reported as a failed
search, not no results; no automatic top-up or vendor switch occurs.

Rollback: revert Work grant handling, its review projection, shared usage,
model guidance and tests together. Historical interactions and optional manifest
usage remain evidence only. Older builds retain explicit-site research and
ignore the additional usage field; they do not recover any public grant.

## Verification and provenance

3 November foreground acceptance: the real Electron review journey approves
one public grant, completes its request, then requires a fresh review and denies
the second request. It passes in 6.0 seconds with a local model fixture and no
external traffic. The related Gmail original-attachment and conversational
follow-up approval journeys pass too. Those checks exposed and fixed a layout
dependency: active review identity is now bound to a host-owned message field
and exact message index, instead of a displayed `Request` row. Custom review
layouts retain the same live interaction ID, token and owner checks.

The opt-in live test `TestLivePublicWebResearch` starts without selected sites,
uses real TinyFish search and public HTTPS, retains an official Go documentation
page, searches its exact captured bytes and returns a cited answer. The retained
report `scripts/competitive-eval/results/research-2026-11-03T07-23-01.419Z.json`
passes 8/8 checks in 19.5 seconds, using five model requests, one search and one
fetch. The earlier failed attempt remains beside it: it reached the correct
source but hit the evaluator's unchanged 128 KiB request bound after raw-page
pagination, including an incorrect local-files search for retained evidence.
Bounded evidence search and path guidance fixed that observed failure.

Run `node scripts/competitive-eval/live-research.mjs` only within the cumulative
authorized allowance. Each attempt reserves all eight possible search charges
before network dispatch. A retained reservation is a conservative ceiling, not
a claim that every reserved query was used or billed. One engineer-authored
question is not a general research-quality or competitor-parity benchmark.

Free production-path tests exercise stale/wrong preview identities, denied/repeated
scope requests, narrow expansion, revoked/cancelled reviews, changed plan state,
later-task isolation, background non-expansion, private DNS, concurrent budgets
and actual manager/custom-worker sharing. Evidence tests check late HTML terms,
bounded contexts/UTF-8 offsets, hash tampering and concurrent immutable reads.

Adapter contracts checked against first-party references on 1 November 2026:

- [TinyFish Search](https://docs.tinyfish.ai/search-api/reference) and
  [pricing](https://www.tinyfish.ai/pricing).
- [Exa Search](https://exa.ai/docs/reference/search).
- [Tavily Search](https://docs.tavily.com/documentation/api-reference/endpoint/search).
- Existing Brave adapter and its retained tests.

These are Gator-owned HTTP adapters, not copied SDK implementations.

```sh
go test ./internal/tools ./internal/workrun -run 'Test.*(WebSearch|WorkSearch|Search|HTTP)' -count=1
go test ./cmd/gator -run 'Test(DoctorReportsConfiguredWebSearch|WebResearchEnvironment|ExecutorReadsWebSearch)' -count=1
```

Both commands pass locally. Fixtures cover all four request/auth contracts,
normalized bounded results, exact-site filtering, configuration ambiguity,
credential separation, 401/402/429/503 handling and secret-bearing transport
errors. These do not prove live account access for all providers.

`TestLiveTinyfishSearch` passed with the operator's configured key: one actual
public query returned three official Go source leads in approximately two
seconds. The report is retained in
`scripts/project-followup-eval/results/tinyfish-2026-11-01-01.json`.
Exa, Tavily and Brave live acceptance have not been performed in this milestone;
their credentials are absent. The multi-step live-model search workflow is
reported separately in the [evaluation record](../scripts/project-followup-eval/README.md).
That workflow also passed with actual TinyFish discovery, official page reads,
project changes and reopened/corrected saved context in attempt 04. One previous
attempt failed at the evaluator's context bound after a large HTML page; the
bounded preview/pagination correction and its regression cases are retained.
