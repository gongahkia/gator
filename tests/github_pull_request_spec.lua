local pull_request = require("gator").module("github").pull_request

local value = pull_request.import({
	number = 152,
	task_id = "task-github-pr",
	pack_id = "pack-github-pr",
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
		if argv[3] == "view" then
			return {
				code = 0,
				stdout = vim.json.encode({
					number = 152,
					title = "Import this pull request",
					body = "Pull request body",
					url = "https://github.com/gongahkia/gator/pull/152",
					state = "OPEN",
					reviewDecision = "APPROVED",
					statusCheckRollup = { { name = "check", conclusion = "SUCCESS" } },
					comments = {
						{
							id = "comment-one",
							body = "unselected",
							url = "https://github.com/gongahkia/gator/pull/152#comment-1",
						},
						{
							id = "comment-two",
							body = "selected",
							url = "https://github.com/gongahkia/gator/pull/152#comment-2",
						},
					},
				}),
			}
		end
		if argv[3] == "diff" then
			return { code = 0, stdout = "diff --git a/a b/a\n+change" }
		end
		error("unexpected GitHub command")
	end,
})
assert(
	value.task.objective == "Import this pull request"
		and #value.task.evidence == 4
		and #value.pack.entries == 4
		and value.pack.entries[1].content:find("Review decision: APPROVED", 1, true)
		and value.pack.entries[2].content:find("diff %-%-git")
		and value.pack.entries[3].content:find("SUCCESS", 1, true),
	"pull request imports must retain diff, review state, and checks as local task context"
)
assert(
	value.pack.entries[4].content == "selected"
		and value.pack.entries[4].trust == "manual"
		and not value.pack.entries[4].transfer.eligible,
	"pull request imports must retain only selected discussions under explicit manual trust"
)
assert(not pcall(pull_request.import, {
	number = 152,
	task_id = "task-github-pr",
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
		if argv[3] == "view" then
			return {
				code = 0,
				stdout = vim.json.encode({
					number = 152,
					title = "pull request",
					body = "",
					url = "https://example.test/152",
					state = "OPEN",
					reviewDecision = "APPROVED",
					statusCheckRollup = {},
					comments = {},
				}),
			}
		end
		return { code = 0, stdout = "" }
	end,
}), "selected pull request comments absent from the GitHub response must fail explicitly")
