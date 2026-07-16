local diff_review = require("gator.ui").diff_review
local run = require("gator.core.run")
local agent_run = run.new({
	id = "run-hunks",
	task_id = "task-hunks",
	provider = { name = "codex", session_id = "native-hunks" },
	process = { pid = 1, executable = "codex" },
	workspace = { kind = "project", root = "/tmp/gator-hunks" },
	state = "completed",
	timing = {},
	usage = {},
})
diff_review.open({
	run = agent_run,
	changes = {
		{
			path = "file.txt",
			before = "one\ntwo\nthree\nfour\nfive\n",
			after = "ONE\ntwo\nthree\nfour\nFIVE\n",
		},
	},
})
local first = diff_review.hunk()
local second = diff_review.next_hunk()
assert(first.id ~= second.id and second.after_start > first.after_start, "native diffs must expose adjacent hunks")
assert(diff_review.previous_hunk().id == first.id, "hunk navigation must move in both directions")
assert(diff_review.stage("accepted").decision == "accepted", "hunk decisions must remain staged")
assert(
	diff_review.annotate("verified manually").annotation == "verified manually",
	"hunks must retain review annotations"
)
local split = diff_review.open_selected()
assert(split.hunk == first.id, "native diff panes must open the selected hunk")
assert(diff_review.close(), "hunk review must close cleanly")
