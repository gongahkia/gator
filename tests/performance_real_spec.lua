local suite = require("gator").module("performance").suite
local pack = require("gator").module("context").pack
local stream = require("gator").module("adapters").stream
local branch = require("gator").module("workspace").branch
local retrieval = require("gator").module("performance").retrieval
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local events = {}
local payload = {}
for index = 1, 100 do
	table.insert(payload, vim.json.encode({ type = "text", index = index }))
end
local report = suite.run({
	cases = {
		startup = function()
			require("gator").setup()
		end,
		context = function()
			pack.new({ id = "perf-pack", task_id = "perf-task", entries = {} })
		end,
		stream = function()
			local parser = stream.new({
				on_event = function(event)
					table.insert(events, event)
				end,
				on_complete = function() end,
			})
			parser:feed(table.concat(payload, "\n") .. "\n")
		end,
		worktree = function()
			branch.resolve({ task_id = "perf-task" })
		end,
		diff = function()
			vim.diff("line one\nline two\n", "line one\nline changed\n")
		end,
		indexer = function()
			retrieval.run({
				queries = { "gator", "context", "provider" },
				lexical = function() end,
				vector = function() end,
			})
		end,
	},
	path = helpers.tempdir("performance-real") .. "/report.json",
})
assert(#report.metrics == 6 and #events == 100, "real performance cases must exercise Gator operations")
