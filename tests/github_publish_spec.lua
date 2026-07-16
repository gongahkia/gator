local publish = require("gator").module("github").publish

local command
local value = publish.review_evidence({
	number = 155,
	confirm = true,
	records = {
		{
			task_id = "task-publish",
			kind = "test",
			command_id = "make-check",
			output_ref = "output token=publish-secret",
			passed = true,
			at = 1,
		},
		{ task_id = "task-publish", kind = "approval", reviewer = "reviewer-one", approved = true, at = 2 },
	},
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
		command = argv
		return { code = 0, stdout = "https://github.com/gongahkia/gator/pull/155#issuecomment-1" }
	end,
})
assert(
	value.published
		and value.task_id == "task-publish"
		and command[1] == "gh"
		and command[2] == "pr"
		and command[3] == "comment"
		and command[4] == "155"
		and command[5] == "--body"
		and command[6]:find("publish%-secret") == nil,
	"confirmed publishing must send only selected, redacted review evidence through gh"
)
assert(not pcall(publish.review_evidence, {
	number = 155,
	records = { { task_id = "task-publish", kind = "reviewer", reviewer = "reviewer-one" } },
	run = function()
		error("publishing must not run before confirmation")
	end,
}), "review evidence publishing must require explicit confirmation before invoking gh")
