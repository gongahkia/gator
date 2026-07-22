local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local overlay = require("gator").module("policy").overlay
local validation = require("gator").module("review").validation

local root = helpers.tempdir("review-validation")
local policy = overlay.new({
	scope = "project",
	target = "project",
	rules = { test_commands = { unit = { argv = { "make", "test" } } } },
	provenance = { source = "project-policy", ref = ".gator/policy.json" },
})
local streamed = {}
local value = validation.execute({
	policy = policy,
	command_id = "unit",
	workspace = { kind = "worktree", root = root },
	run = function(argv, cwd, emit)
		assert(
			argv[1] == "make" and argv[2] == "test" and cwd == vim.uv.fs_realpath(root),
			"approved argv must run in the selected workspace"
		)
		emit("stdout", "tests passed\n")
		emit("stderr", "warning\n")
		return { code = 0 }
	end,
	on_evidence = function(event)
		table.insert(streamed, event)
	end,
})
assert(
	value.passed
		and #value.evidence == 2
		and streamed[2].stream == "stderr"
		and value.policy.provenance.source == "project-policy",
	"validation must stream policy-provenanced command evidence"
)
local failed = validation.execute({
	policy = policy,
	command_id = "unit",
	workspace = { kind = "worktree", root = root },
	run = function(_, _, emit)
		emit("stderr", "test failure\n")
		return { code = 2 }
	end,
})
assert(
	not failed.passed and failed.code == 2 and failed.evidence[1].stream == "stderr",
	"nonzero policy-approved validations must retain explicit failure evidence"
)
assert(
	not pcall(
		validation.execute,
		{ policy = policy, command_id = "shell", workspace = { kind = "worktree", root = root } }
	),
	"unapproved commands must not execute"
)
