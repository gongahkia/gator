local config = require("gator.config")
local state = require("gator.state")
local task_file = require("gator.core.task_file")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("workflow")
local opened, attached = {}, false
local terminal = {
	open = function(_, opts)
		table.insert(opened, opts)
		return { id = opts.id }
	end,
	attach = function()
		if attached then
			return 1
		end
		error("not attached")
	end,
}
local bridge = {
	start = function(_, opts, callback)
		callback({
			session = { provider = opts.provider, id = "native-session", owner = "provider" },
			command = { opts.provider, opts.prompt },
		})
	end,
	resume = function(_, opts, callback)
		callback({ session = opts.session, command = { opts.provider, "resume", opts.session.id } })
	end,
}
local current = state.new(config.resolve({ providers = { pi = { user_confirmed = true } } }), { supported = true })
local value = workflow.new({
	state = current,
	root = root,
	terminal = terminal,
	bridge = bridge,
	readiness = function(opts)
		assert(opts.pi_user_confirmed, "workflow must pass the explicit Pi confirmation to provider readiness")
		return {
			{ provider = "claude", available = true },
			{ provider = "pi", available = true, authentication = "user_confirmed" },
		}
	end,
})

local created = value:create("Review the local workflow")
assert(
	created.id == "review-the-local-workflow"
		and vim.fn.filereadable(root .. "/.gator/tasks/review-the-local-workflow.md") == 1
		and current.active_task_id == created.id,
	"task creation must persist a selected project-local Markdown task"
)
assert(
	vim.tbl_contains(require("gator.ui.palette").complete(""), "action:create-task")
		and vim.tbl_contains(require("gator.ui.palette").complete(""), "task:" .. created.id)
		and vim.tbl_contains(require("gator.ui.palette").complete(""), "provider:claude")
		and vim.tbl_contains(require("gator.ui.palette").complete(""), "provider:pi"),
	"workflow setup must register built-in, task, and ready-provider palette entries"
)

value:launch("claude")
local running = value:task(created.id)
assert(
	#opened == 1
		and opened[1].command[1] == "claude"
		and opened[1].command[2] == created.objective
		and running.lifecycle == "running"
		and running.sessions[1].id == "native-session",
	"launch must link the native provider session and open the prepared terminal command"
)
opened[1].on_exit({ code = 0 })
assert(
	value:task(created.id).lifecycle == "awaiting_review"
		and task_file.parse(table.concat(vim.fn.readfile(root .. "/.gator/tasks/" .. created.id .. ".md"), "\n")).lifecycle
			== "awaiting_review",
	"successful terminal exit must persist review-ready task state"
)

value:attach()
assert(
	#opened == 2 and opened[2].command[2] == "resume",
	"detached provider sessions must resume through the native bridge"
)
attached = true
assert(value:attach(), "open terminal sessions must attach without relaunching")

local imported = value:load()
assert(imported.tasks == 1 and #imported.failures == 0, "Markdown task import must reload valid project-local tasks")
value:close()
