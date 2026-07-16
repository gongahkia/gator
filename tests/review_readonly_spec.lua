local adapters = require("gator").module("adapters")
local overlay = require("gator").module("policy").overlay
local readonly = require("gator").module("review").readonly
local task = require("gator").module("core").task
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local supported = { available = true, modes = { "native" } }
local contract = adapters.capabilities.new({
	provider = "codex",
	transport = supported,
	auth = supported,
	session = supported,
	permission = { available = true, modes = { "sandbox", "approval" } },
	model = supported,
	command = supported,
	tool = supported,
	context = supported,
	usage = supported,
})
local root = helpers.tempdir("review-readonly")
local entity = task.new({
	id = "task-readonly",
	objective = "Review a task diff",
	workspace = { kind = "worktree", root = root },
	created_at = 1,
	updated_at = 1,
})
local policy = overlay.new({
	scope = "run",
	target = "review-readonly",
	rules = { write_allowed = false },
	provenance = { source = "policy", ref = "test" },
})
local value = readonly.launch({
	id = "review-one",
	task = entity,
	diff = { root = root, base = "base-sha", patch = "diff --git a/file b/file\n" },
	policy = policy,
	capabilities = contract,
	map_policy = adapters.codex_policy.map,
	is_read_only = function(mapped)
		return mapped.sandbox_mode == "read-only"
	end,
	launch = function(request)
		assert(
			request.review.base_revision == "base-sha" and request.policy.sandbox_mode == "read-only",
			"reviewer must receive diff and native read-only policy"
		)
		return { id = request.id, state = "running" }
	end,
})
assert(
	value.state == "running" and value.task_id == "task-readonly",
	"read-only reviewer launches must retain task identity"
)
local unsafe = overlay.new({
	scope = "run",
	target = "review-readonly",
	rules = { write_allowed = true },
	provenance = { source = "policy", ref = "test" },
})
assert(not pcall(readonly.launch, {
	id = "review-two",
	task = entity,
	diff = { root = root, base = "base", patch = "diff" },
	policy = unsafe,
	capabilities = contract,
	map_policy = adapters.codex_policy.map,
	is_read_only = function()
		return true
	end,
	launch = function()
		return { id = "review-two", state = "running" }
	end,
}), "review launch must refuse write-capable policies")
