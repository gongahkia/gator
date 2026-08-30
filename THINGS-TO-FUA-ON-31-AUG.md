# THINGS TO FOLLOW UP ON

4. a prior default-parallel go test ./... run intermittently failed
  unchanged internal/appserver terminal timing test; its exact test passed five
  isolated runs. [Inference] This is an existing test flake unrelated to the browser
  changes, but normal CI may still reproduce it.

  5. Extensibility is safer but much narrower than the market.

     Gator profiles can only reduce authority; its extension/MCP/LSP model is guarded and pinned. That is a good security choice, but it cannot match the programmable
     ecosystem of Claude Code custom subagents/hooks/skills, OpenCode’s per-agent tool permissions, or Pi’s TypeScript extensions, packages, custom providers, and UI. Claude
     Code subagents, OpenCode agents, Pi packages

  6. Release and project hygiene are not ready.
      - No local release tags; gator version reports dev (none, unknown).
      - No tracked LICENSE, SECURITY.md, contributor guidance, dependency-update bot, SBOM, vulnerability scan, signed artifact, or provenance step.
      - The installer verifies checksums, which is good, but the release workflow does not sign artifacts or produce provenance.
      - The current committed tree fails its own CI formatting check: gofmt -l cmd internal reports internal/journal/journal.go. This is a small defect, but it means the
        published CI gate would currently fail.

7.  Coverage is meaningful but uneven: agent loop 86.0%, app server 80.1%, run executor 74.3%, terminal 71.9%, tools 69.9%, sandbox 63.7%, TUI 61.8%; CLI entrypoint is only
  45.6% and eval is 39.6%. There are two fuzz targets, both in the app-server HTTP/parser area, and no benchmarks.

8. These other below scattered issues

  - No full race-detector run.
  - No fuzzing campaign.
  - No live-provider tests; provider tests use local HTTP mocks.
  - No broad Linux sandbox-escape regression test—the explicit “cannot write outside worktree” test is macOS-only.
  - No real browser, LSP, MCP, OAuth, or multi-agent dogfood evidence against external systems.
  - No real-model evaluation or comparative benchmark.
