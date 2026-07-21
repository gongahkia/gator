local suite = require("gator").module("performance").suite
local budgets = require("gator").module("performance").budgets
local pack = require("gator").module("context").pack
local automatic = require("gator").module("context").automatic
local stream = require("gator").module("adapters").stream
local worktree = require("gator").module("workspace").worktree
local diff_review = require("gator").module("ui").diff_review
local timeline = require("gator").module("ui").timeline
local core_run = require("gator").module("core").run
local retrieval = require("gator").module("performance").retrieval
local protocol = require("gator").module("indexer").protocol
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local entries = {}
for index = 1, 256 do
	table.insert(entries, {
		id = string.format("perf-entry-%03d", index),
		kind = "file",
		ref = string.format("lua/gator/perf-%03d.lua", index),
		provenance = { source = "fixture", ref = "tests/fixtures/sample.txt" },
		trust = "provenance",
		token_estimate = { status = "estimated", tokens = 64 },
		transfer = {
			eligible = index % 3 ~= 0,
			reason = index % 3 == 0 and "fixture policy excludes this entry" or nil,
		},
		content = string.rep("fixture context ", 16),
	})
end
local fixture = helpers.read(helpers.fixture_path("adapters/stream.jsonl"))
local event = assert(fixture:match("[^\n]+"), "stream fixture must contain an event")
local payload = {}
for _ = 1, 5000 do
	table.insert(payload, event)
end
table.insert(payload, vim.json.encode({ type = "complete", reason = "fixture" }))
local stream_payload = table.concat(payload, "\n") .. "\n"
local events = 0
local ui_updates = 0
local worktree_root = helpers.tempdir("performance-worktree-root")
local worktree_parent = helpers.tempdir("performance-worktree-parent")
local before, after = {}, {}
for index = 1, 1200 do
	table.insert(before, "local fixture_" .. index .. " = " .. index)
	table.insert(after, "local fixture_" .. index .. " = " .. (index % 40 == 0 and index + 1 or index))
end
local review_run = core_run.new({
	id = "performance-review",
	task_id = "performance-task",
	provider = { name = "fixture" },
	process = { pid = 1, executable = "fixture" },
	workspace = { kind = "project", root = vim.g.gator_test.root },
	state = "completed",
	timing = {},
	usage = {},
})
local corpus = {}
for index = 1, 256 do
	table.insert(corpus, helpers.read(helpers.fixture_path("sample.txt")) .. " gator context provider " .. index)
end
local artifact = (vim.env.GATOR_TEST_ROOT ~= "" and vim.env.GATOR_TEST_ROOT or helpers.tempdir("performance-real"))
	.. "/benchmark-report.json"
local report = suite.run({
	samples = budgets.samples,
	budgets = budgets.values,
	cases = {
		startup = function()
			require("gator").setup()
		end,
		context = function()
			local value = pack.new({ id = "performance-pack", task_id = "performance-task", entries = entries })
			automatic.attach({
				pack = value,
				policy = function(entry)
					return { allowed = entry.id:sub(-1) ~= "0", reason = "fixture context policy" }
				end,
			})
		end,
		stream = function()
			local parser = stream.new({
				on_event = function()
					events = events + 1
				end,
				on_complete = function() end,
			})
			parser:feed(stream_payload)
		end,
		ui_loop = function()
			timeline.open({
				calls = {
					{
						id = "performance-stream-call",
						provider = "fixture",
						session_id = "performance-stream-session",
						name = "stream_update",
						arguments = "{}",
						approval = "not_required",
						output = "chunk 0",
						status = "running",
					},
				},
			})
			for index = 1, 2000 do
				timeline.update({
					{
						id = "performance-stream-call",
						provider = "fixture",
						session_id = "performance-stream-session",
						name = "stream_update",
						arguments = "{}",
						approval = "not_required",
						output = "chunk " .. index,
						status = "running",
					},
				})
				ui_updates = ui_updates + 1
			end
			assert(
				vim.wait(1000, function()
					local value = timeline.inspect()
					return value and not value.refresh_pending
				end),
				"benchmark must drain coalesced timeline updates"
			)
			assert(timeline.close(), "benchmark must cancel its timeline")
		end,
		worktree = function()
			worktree.create({
				root = worktree_root,
				path = worktree_parent .. "/performance-worktree",
				branch = "gator/performance-worktree",
				base = "HEAD",
				run = function(argv)
					assert(argv[2] == "worktree" and argv[3] == "add", "benchmark must exercise worktree creation")
					return { code = 0 }
				end,
			})
		end,
		diff = function()
			diff_review.open({
				run = review_run,
				changes = {
					{ path = "fixture.lua", before = table.concat(before, "\n"), after = table.concat(after, "\n") },
				},
			})
			diff_review.open_selected()
			assert(diff_review.close(), "benchmark must close rendered diff views")
		end,
		indexer = function()
			for index = 1, 128 do
				protocol.request("performance-index-" .. index, "index", { path = corpus[index] })
			end
			retrieval.run({
				queries = { "gator", "context", "provider", "fixture" },
				lexical = function(query)
					for _, document in ipairs(corpus) do
						document:find(query, 1, true)
					end
				end,
				vector = function(query)
					for _, document in ipairs(corpus) do
						vim.fn.sha256(query .. document)
					end
				end,
			})
		end,
	},
	path = artifact,
})
assert(
	#report.metrics == 7
		and #report.regressions == 0
		and events == 15000
		and ui_updates == 6000
		and vim.fn.filereadable(artifact) == 1,
	"real performance cases must exercise fixture-backed Gator operations and emit a regression artifact"
)
