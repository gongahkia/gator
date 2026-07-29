local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local config = require("gator.config")

assert(config.resolve().schema_version == config.schema_version, "default settings must resolve to the current schema")
assert(config.resolve().ui.icons == "unicode", "Unicode alligator glyphs must be the default UI style")
assert(
	config.resolve().ui.ask_selection.keymap == "<leader>gA" and config.resolve().launch.stall_after_ms == 120000,
	"selection asks and non-destructive provider-stall warnings must have safe defaults"
)

local path = helpers.tempdir("config") .. "/gator.json"
helpers.write(path, '{"schema_version":2,"ui":{"layout":"modal"},"workspaces":{"mode":"worktree","max_write_runs":2}}')
local value = config.load(path)
assert(
	value.schema_version == config.schema_version
		and value.ui.layout == "modal"
		and value.workspaces.mode == "worktree"
		and value.workspaces.mode == "worktree",
	"global settings must load user defaults"
)
assert(
	not pcall(config.resolve, { telemetry = { token = "secret" } }),
	"global settings must reject provider credentials"
)
helpers.write(path, "not-json")
assert(not pcall(config.load, path), "invalid user defaults must fail explicitly")
assert(
	config.resolve({ schema_version = 1 }).schema_version == config.schema_version,
	"legacy configuration schemas must migrate in memory"
)
assert(not pcall(config.resolve, { schema_version = "2" }), "configuration schemas must require an integer version")
local handoff = config.resolve({ context = { handoff = { author = "gator", max_chars = 2048 } } }).context.handoff
assert(
	handoff.author == "gator" and handoff.max_chars == 2048 and handoff.review == "required",
	"handoff authoring settings must inherit required review enforcement"
)
local optional_handoff = config.resolve({
	context = { handoff = { author = "gator", max_chars = 2048, review = "optional" } },
}).context.handoff
assert(optional_handoff.review == "optional", "handoff authoring settings must resolve optional review enforcement")
assert(
	config.resolve({ providers = { pi = { user_confirmed = true } } }).providers.pi.user_confirmed,
	"Pi launch must require an explicit local user confirmation"
)
local managed_provider = config.resolve({ providers = { gemini = { user_confirmed = true } } }).providers
assert(
	managed_provider.gemini.user_confirmed and not managed_provider.copilot.user_confirmed,
	"managed providers must require an explicit per-provider local confirmation"
)
local loading = config.resolve({ ui = { loading = { spinner = "whirly.hanoi", interval_ms = 80 } } }).ui.loading
assert(
	loading.enabled and loading.spinner == "whirly.hanoi" and loading.interval_ms == 80,
	"loading configuration must select a bundled spinner and optional cadence override"
)
local resources =
	config.resolve({ ui = { resources = { enabled = true, fields = { "wall_time", "usage" } } } }).ui.resources
assert(
	resources.enabled and #resources.fields == 2 and resources.fields[2] == "usage",
	"resource display must default on and allow an explicit field subset"
)
local chat = config.resolve({ ui = { chat = { layout = "float", height = 20, width = 90 } } }).ui.chat
assert(
	chat.layout == "float" and chat.height == 20 and chat.width == 90,
	"chat layout settings must support float, height, and width"
)
local ask_selection = config.resolve({ ui = { ask_selection = { keymap = false } } }).ui.ask_selection
assert(ask_selection.keymap == false, "selection ask mapping must allow an explicit opt-out")
local edit_settings = config.resolve({
	ui = { composer = { enabled = false }, edit_selection = { keymap = false } },
	context = { references = { roots = {}, max_files = 2, max_file_bytes = 1024, max_total_bytes = 2048 } },
	edits = { save = "ask" },
})
assert(
	not edit_settings.ui.composer.enabled
		and edit_settings.ui.edit_selection.keymap == false
		and edit_settings.context.references.max_files == 2
		and edit_settings.edits.save == "ask",
	"composer, references, selection-edit mapping, and save policy must be configurable"
)
assert(
	config.resolve({ launch = { stall_after_ms = 120 } }).launch.stall_after_ms == 120,
	"provider stall threshold must be configurable in milliseconds"
)
local extensions = config.resolve({ extensions = { modules = { "my_gator_extension" } } }).extensions
assert(extensions.modules[1] == "my_gator_extension", "extensions must require explicit trusted module names")
local ui_extensions = config.resolve({
	ui = {
		renderers = { provider_picker = "my-picker" },
		run_graph = { columns = { "id", "my-column" } },
	},
}).ui
assert(
	ui_extensions.renderers.provider_picker == "my-picker" and ui_extensions.run_graph.columns[2] == "my-column",
	"UI extension selection and graph columns must be configurable"
)
local budget = config.resolve({ budget = { max_tokens = 1000, action = "stop", max_concurrent_runs = 2 } }).budget
assert(
	budget.max_tokens == 1000 and budget.action == "stop" and budget.max_concurrent_runs == 2,
	"budget configuration must preserve explicit limits and actions"
)
local permissions = config.resolve({ permissions = { codex = { sandbox = "read_only" } } }).permissions
assert(
	permissions.codex.sandbox == "read_only",
	"Codex launch policy must accept an explicit read-only App Server sandbox"
)
local retention = config.resolve({ retention = { max_age_days = 90, cleanup_on_start = false } }).retention
assert(
	retention.max_age_days == 90
		and retention.max_bytes == 0
		and not retention.cleanup_on_start
		and retention.worktrees == "inactive_clean",
	"retention must accept an arbitrary fixed period and preserve safe worktree cleanup"
)
local preflight = config.resolve({ context = { preflight = { confirm = true } } }).context.preflight
assert(preflight.confirm, "context preflight confirmation must be an explicit opt-in")
assert(
	config.resolve({ retention = { max_bytes = 4096 } }).retention.max_bytes == 4096,
	"retention must accept an explicit local artifact quota"
)
local review = config.resolve({ review = { commands = { unit = { argv = { "make", "test" } } } } }).review
assert(review.commands.unit.argv[2] == "test", "review commands must be explicit argv arrays")
local acp = config.resolve({
	acp = { commands = { localagent = { argv = { "local-agent", "--acp" } } } },
	launch = { default_provider = "localagent" },
}).acp
assert(acp.commands.localagent.argv[1] == "local-agent", "ACP commands must require an explicit configured argv")
assert(
	not pcall(
			config.resolve,
			{ context = { handoff = { author = "invalid", max_chars = 1, review = "required" } } }
		)
		and not pcall(
			config.resolve,
			{ context = { handoff = { author = "user", max_chars = 0, review = "required" } } }
		)
		and not pcall(
			config.resolve,
			{ context = { handoff = { author = "user", max_chars = 1, review = "invalid" } } }
		)
		and not pcall(config.resolve, { providers = { pi = { user_confirmed = "yes" } } })
		and not pcall(config.resolve, { providers = { unknown = { user_confirmed = true } } })
		and not pcall(config.resolve, { ui = { loading = { spinner = "unknown" } } })
		and not pcall(config.resolve, { ui = { loading = { interval_ms = 15 } } })
		and not pcall(config.resolve, { ui = { chat = { layout = "side" } } })
		and not pcall(config.resolve, { ui = { chat = { height = 5 } } })
		and not pcall(config.resolve, { ui = { chat = { width = 19 } } })
		and not pcall(config.resolve, { ui = { ask_selection = { keymap = true } } })
		and not pcall(config.resolve, { ui = { edit_selection = { keymap = true } } })
		and not pcall(config.resolve, { ui = { composer = { enabled = "yes" } } })
		and not pcall(config.resolve, { edits = { save = "now" } })
		and not pcall(config.resolve, { context = { references = { max_files = 0 } } })
		and not pcall(config.resolve, { ui = { ask_selection = { keymap = "" } } })
		and not pcall(config.resolve, { ui = { resources = { enabled = "yes" } } })
		and not pcall(config.resolve, { ui = { resources = { fields = { "tokens" } } } })
		and not pcall(config.resolve, { ui = { resources = { fields = { "usage", "usage" } } } })
		and not pcall(config.resolve, { extensions = { modules = { "invalid module" } } })
		and not pcall(config.resolve, { ui = { run_graph = { columns = { "id", "id" } } } })
		and not pcall(config.resolve, { permissions = { codex = { sandbox = "unrestricted" } } })
		and not pcall(config.resolve, { budget = { max_tokens = -1 } })
		and not pcall(config.resolve, { launch = { stall_after_ms = -1 } })
		and not pcall(config.resolve, { retention = { max_age_days = -1 } })
		and not pcall(config.resolve, { retention = { max_bytes = -1 } })
		and not pcall(config.resolve, { retention = { cleanup_on_start = "yes" } })
		and not pcall(config.resolve, { context = { preflight = { confirm = "yes" } } })
		and not pcall(config.resolve, { review = { commands = { invalid = { argv = {} } } } })
		and not pcall(config.resolve, { acp = { commands = { Invalid = { argv = { "agent" } } } } })
		and not pcall(config.resolve, { budget = { action = "invalid" } }),
	"handoff authoring settings must reject unsupported authors, bounds, and review modes"
)

helpers.write(path, '{"schema_version":1,"ui":{"layout":"modal"}}')
local migrated, migrated_provenance, migrations = config.load(path)
assert(
	migrated.schema_version == config.schema_version
		and migrated.ui.layout == "modal"
		and migrations[1].from_version == 1
		and migrations[1].to_version == config.schema_version
		and migrated_provenance.schema_version.source == "migration",
	"legacy configuration files must migrate to schema v3 with provenance"
)
local legacy = '{"context":{"mode":"inspect"}}'
helpers.write(path, legacy)
assert(
	config.load(path).schema_version == config.schema_version
		and config.load(path).context.mode == "inspect"
		and table.concat(vim.fn.readfile(path), "\n") == legacy,
	"unversioned legacy configuration files must migrate without a durable rewrite"
)
helpers.write(path, '{"schema_version":16}')
assert(not pcall(config.load, path), "unknown file schemas must fail before migration")

local layered = config.resolve_sources({
	{
		source = "setup",
		ref = "gator.setup",
		settings = { ui = { layout = "modal" } },
	},
	{
		source = "file",
		ref = path,
		settings = { ui = { layout = "adaptive", screen_reader = false } },
	},
})
assert(
	layered.settings.ui.layout == "modal"
		and not layered.settings.ui.screen_reader
		and layered.provenance["ui.layout"].source == "setup"
		and layered.provenance["ui.screen_reader"].source == "file"
		and layered.provenance["context.handoff.author"].source == "defaults"
		and layered.provenance["context.mode"].source == "defaults",
	"configuration layers must use deterministic precedence with field provenance"
)
assert(not pcall(config.resolve_sources, {
	{ source = "file", ref = path, settings = {} },
	{ source = "file", ref = path .. ".override", settings = {} },
}), "configuration layers must reject duplicate sources")

local ok, diagnostic = pcall(config.resolve_sources, {
	{ source = "token: private-value", ref = path, settings = {} },
})
assert(
	not ok and not diagnostic:find("private%-value"),
	"configuration diagnostics must redact sensitive source values before callers receive them"
)
