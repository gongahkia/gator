local issue = require("gator").module("github").issue

local query
local value = issue.import({
	number = 150,
	task_id = "task-github-issue",
	pack_id = "pack-github-issue",
	comment_ids = { "comment-two" },
	at = 1,
	run = function(argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = "gh version 2.73.0" }
		end
		if
			argv[2] == "auth"
			or (argv[2] == "issue" and argv[3] == "--help")
			or (argv[2] == "pr" and argv[3] == "--help")
		then
			return { code = 0, stdout = "" }
		end
		query = argv
		return {
			code = 0,
			stdout = vim.json.encode({
				number = 150,
				title = "Import this GitHub issue",
				body = "Issue body",
				url = "https://github.com/gongahkia/gator/issues/150",
				labels = { { name = "type:feature" }, { name = "area:github" } },
				comments = {
					{
						id = "comment-one",
						body = "unselected",
						url = "https://github.com/gongahkia/gator/issues/150#issuecomment-1",
					},
					{
						id = "comment-two",
						body = "selected",
						url = "https://github.com/gongahkia/gator/issues/150#issuecomment-2",
					},
				},
			}),
		}
	end,
})
assert(
	query[1] == "gh"
		and query[2] == "issue"
		and query[3] == "view"
		and query[4] == "150"
		and value.task.objective == "Import this GitHub issue"
		and #value.task.evidence == 2,
	"issue imports must query the selected issue and create local task evidence"
)
assert(
	#value.pack.entries == 2
		and value.pack.entries[1].content:find("Issue body", 1, true)
		and value.pack.entries[1].content:find("type:feature", 1, true)
		and value.pack.entries[2].content == "selected"
		and value.pack.entries[2].trust == "manual"
		and not value.pack.entries[2].transfer.eligible,
	"issue imports must retain body, labels, and only selected comments under explicit manual trust"
)
assert(
	value.pack.entries[1].token_estimate.status == "unavailable" and not value.pack.entries[1].transfer.eligible,
	"imported GitHub issue context must remain unavailable for automatic transfer"
)
assert(not pcall(issue.import, {
	number = 150,
	task_id = "task-github-issue",
	comment_ids = { "missing" },
	run = function(argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = "gh version 2.73.0" }
		end
		if
			argv[2] == "auth"
			or (argv[2] == "issue" and argv[3] == "--help")
			or (argv[2] == "pr" and argv[3] == "--help")
		then
			return { code = 0, stdout = "" }
		end
		return {
			code = 0,
			stdout = vim.json.encode({
				number = 150,
				title = "issue",
				body = "",
				url = "https://example.test/150",
				labels = {},
				comments = {},
			}),
		}
	end,
}), "selected comments absent from the GitHub response must fail explicitly")
