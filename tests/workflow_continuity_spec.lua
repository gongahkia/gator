local config = require("gator.config")
local conversation = require("gator.ui.conversation")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("workflow-continuity")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
local opened = nil
local value = workflow.new({
	state = state.new(config.resolve({ providers = { pi = { user_confirmed = true } } }), { supported = true }),
	root = root,
	readiness = function()
		return { { provider = "pi", available = true, readiness_state = "user_confirmed" } }
	end,
	structured = {
		open = function(_, opts)
			opened = opts
			opts.on_session({ id = opts.session.id, path = opts.session.path, resume_supported = true })
			return {
				send = function()
					return true
				end,
				cancel = function()
					return true
				end,
				stop = function()
					return true
				end,
			}
		end,
	},
	loading = {
		open = function()
			return { close = function() end }
		end,
	},
})
value:put({
	id = "run-continuity",
	provider = "pi",
	role = "primary",
	transport = "chat",
	state = "waiting_input",
	workspace = { kind = "project", root = root },
	bundle_id = "bundle-continuity",
	objective = "Continue the native session",
	transcript = "available",
	usage = { state = "unknown", context_tokens_estimate = 1 },
	budget = { limit_tokens = 0, action = "warn", state = "unbounded" },
	session = { id = "pi-session", path = "/tmp/pi-session.jsonl", resume_supported = true },
	created_at = 1,
	updated_at = 1,
})
value.store:transcript("run-continuity", "## user\nOriginal question\n\n## assistant\nOriginal answer")
assert(
	value:recover() == 1 and value:run("run-continuity").state == "detached",
	"startup recovery must detach prior live runs"
)
assert(value:resume("run-continuity"), "detached structured runs must resume through their provider contract")
assert(
	opened.operation == "resume"
		and opened.session.id == "pi-session"
		and opened.prompt == nil
		and value:run("run-continuity").state == "waiting_input",
	"chat resume must reopen the same provider session without starting a synthetic turn"
)
local content = table.concat(vim.api.nvim_buf_get_lines(0, 0, -1, false), "\n")
assert(
	content:find("Original question", 1, true) and content:find("Original answer", 1, true),
	"resumed chats must show their persisted Gator transcript"
)
assert(conversation.close(), "continuity chat must remain ephemeral")
