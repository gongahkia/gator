local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("run-workflow")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "test project must initialize Git")

vim.cmd("enew!")
local buffer = vim.api.nvim_get_current_buf()
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "local selected = true", "return selected" })
local selection = require("gator.context.capture").current({ buffer = buffer, first_line = 1, last_line = 1 })
local opened = {}
local current = state.new(config.resolve({ providers = { pi = { user_confirmed = true } } }), { supported = true })
local value = workflow.new({
	state = current,
	root = root,
	readiness = function()
		return { { provider = "pi", available = true, readiness_state = "user_confirmed" } }
	end,
	bridge = {
		start = function(_, opts, callback)
			callback({ session = { provider = opts.provider, id = "pi-session", owner = "provider" }, command = { "pi", opts.prompt } })
		end,
		resume = function(_, opts, callback)
			callback({ session = opts.session, command = { "pi", "--session", opts.session.id } })
		end,
	},
	terminal = {
		open = function(_, opts)
			table.insert(opened, opts)
			return { job_id = 1 }
		end,
		attach = function()
			return true
		end,
		close = function()
			return true
		end,
	},
})

local run = value:launch({ provider = "pi", transport = "terminal", objective = "Fix selected code", capture = selection })
local persisted = value:run(run.id)
assert(
	persisted.state == "running"
		and persisted.transport == "terminal"
		and persisted.transcript == "unavailable"
		and persisted.usage.state == "estimated",
	"launch must create a run-first terminal record with explicit transcript and usage state"
)
assert(
	#opened == 1 and opened[1].command[2]:find("local selected = true", 1, true),
	"native launch must receive the captured source rather than only the objective"
)
assert(
	vim.fn.filereadable(root .. "/.gator/runs/" .. run.id .. ".json") == 1
		and table.concat(vim.fn.readfile(root .. "/.git/info/exclude"), "\n"):find(".gator/", 1, true),
	"run records must be project-local and locally ignored"
)
assert(value.store:project().default_provider == "pi", "successful launch must remember the project provider")
assert(value:resume(run.id), "a live terminal must focus its existing Neovim terminal before relaunching")
assert(value:stop(run.id) and value:run(run.id).state == "stopped", "stopping must update the run graph state")
