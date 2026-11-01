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
Work additionally requires explicitly selected HTTPS origins. Its search wrapper
adds site operators and independently filters returned URLs by exact selected
scheme/host/port; operators are relevance hints, not authorization. Credential
URLs and off-scope results are omitted. Search snippets are untrusted discovery
leads and do not become factual evidence. `http_fetch` retains the actual selected
page before its contents can be cited as checked evidence. There is no fetch or
browser/agent fallback to TinyFish or another vendor.

Work returns at most 8 KiB of readable page preview to the model. HTML previews
omit script/style/navigation/footer text and normalize visible text; they do not
execute scripts. Exact fetched bytes and their digest remain in the evidence
catalog. `read_evidence` provides bounded raw-byte pages using `offset_bytes`,
`limit_bytes` (at most 32 KiB) and `next_offset_bytes`; UTF-8 boundaries are checked.
Truncation and preview normalization are explicit. Exact quote checks still use
the original bytes, so a normalized preview is not evidence of a literal match
across HTML tags. No full-page completeness or JavaScript rendering is claimed.

All adapters normalize bounded title/URL/snippet/date metadata. At most ten
results are returned, fields and response bodies are bounded, and search/fetch
share the existing eight-request run budget. Production search uses the existing
HTTP deadline and public-address connection checks. Fixed provider endpoints
receive keys through headers only. Raw provider error bodies and transport
errors are not exposed; errors retain HTTP status where available.

Tavily explicitly requests `basic` depth, disables automatic parameter selection,
answers, raw content and images. Exa uses standard `auto` search and requests no
extra content/answer/research endpoint. TinyFish uses Search only, never its paid
Agent, Browser or Monitor APIs. Provider pricing, free allowances and account
limits remain provider-specific; changing search provider does not change the
configured language model or its API charges.

TinyFish's pricing page describes free Search, while its API documentation also
describes a daily allowance and possible wallet behavior beyond it. Do not infer
unlimited free service from the marketing page. A 402/429 is reported as a failed
search, not no results; no automatic top-up or vendor switch occurs.

## Verification and provenance

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
