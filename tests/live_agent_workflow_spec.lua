local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local workflow = helpers.read(vim.g.gator_test.root .. "/.github/workflows/live-agent-e2e.yml")

assert(workflow:find("workflow_dispatch:", 1, true), "live-agent workflow must be manually dispatched")
assert(
	not workflow:find("pull_request:", 1, true) and not workflow:find("push:", 1, true),
	"live-agent workflow must not run on repository events"
)
assert(
	workflow:find("runs-on: [self-hosted, gator-live-agents]", 1, true),
	"live-agent workflow must use the dedicated trusted runner"
)
assert(
	workflow:find("environment: protected-live-agents", 1, true),
	"live-agent workflow must require the protected environment"
)
assert(
	workflow:find("github.ref == format('refs/heads/{0}', github.event.repository.default_branch)", 1, true),
	"live-agent workflow must reject non-default branches"
)
assert(
	workflow:find("contents: read", 1, true) and not workflow:find("secrets.", 1, true),
	"live-agent workflow must use least-privilege permissions without exporting secrets"
)
assert(
	workflow:find("actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5", 1, true),
	"live-agent workflow must pin checkout to an immutable commit SHA"
)
for _, target in ipairs({
	"make live-aider-e2e",
	"make live-amp-e2e",
	"make live-cline-e2e",
	"make live-cursor-e2e",
	"make live-codex-test",
	"make live-claude-test",
	"make live-droid-e2e",
	"make live-gemini-e2e",
	"make live-goose-e2e",
	"make live-kimi-e2e",
	"make live-vibe-e2e",
	"make live-copilot-e2e",
	"make live-opencode-e2e",
	"make live-pi-e2e",
	"make live-handoff-e2e",
}) do
	assert(workflow:find(target, 1, true), "live-agent workflow must route the provider target: " .. target)
end
