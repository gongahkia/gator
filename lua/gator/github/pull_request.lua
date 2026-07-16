local gh = require("gator.github.gh")
local pack = require("gator.context.pack")
local task = require("gator.core.task")
local M = {}

local function fail(message)
	error("Gator GitHub pull request import: " .. message, 3)
end

local function text(value, name, allow_empty)
	if type(value) ~= "string" or (not allow_empty and value == "") then
		fail(name .. " must be " .. (allow_empty and "a string" or "a non-empty string"))
	end
	return value
end

local function number(value)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 then
		fail("number must be a positive integer")
	end
	return value
end

local function identifier(value, name)
	value = text(value, name)
	if not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function invoke(run, argv, cwd, name)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or type(result.code) ~= "number" or result.code % 1 ~= 0 then
		fail(name .. " failed")
	end
	if result.stdout ~= nil and type(result.stdout) ~= "string" then
		fail(name .. " returned invalid stdout")
	end
	return { code = result.code, stdout = result.stdout or "" }
end

local function selected(ids)
	if ids == nil then
		return {}
	end
	if type(ids) ~= "table" or not vim.islist(ids) then
		fail("comment_ids must be a list")
	end
	local result = {}
	for index, value in ipairs(ids) do
		value = text(value, "comment_ids[" .. index .. "]")
		if result[value] then
			fail("comment_ids must not repeat values")
		end
		result[value] = true
	end
	return result
end

local function checks(value)
	if value == nil or value == vim.NIL then
		return {}
	end
	if type(value) ~= "table" or not vim.islist(value) then
		fail("GitHub pull request checks must be a list")
	end
	return vim.deepcopy(value)
end

local function document(value)
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail("GitHub pull request response must be an object")
	end
	for key in pairs(value) do
		if
			key ~= "number"
			and key ~= "title"
			and key ~= "body"
			and key ~= "url"
			and key ~= "state"
			and key ~= "reviewDecision"
			and key ~= "statusCheckRollup"
			and key ~= "comments"
		then
			fail("GitHub pull request response contains unsupported field: " .. tostring(key))
		end
	end
	number(value.number)
	text(value.title, "GitHub pull request title")
	text(value.body, "GitHub pull request body", true)
	text(value.url, "GitHub pull request URL")
	text(value.state, "GitHub pull request state")
	local review = (value.reviewDecision == nil or value.reviewDecision == vim.NIL) and "unavailable"
		or text(value.reviewDecision, "GitHub review decision")
	if type(value.comments) ~= "table" or not vim.islist(value.comments) then
		fail("GitHub pull request comments must be a list")
	end
	local comments = {}
	for index, comment in ipairs(value.comments) do
		if type(comment) ~= "table" then
			fail("GitHub pull request comment " .. index .. " must be an object")
		end
		comments[index] = {
			id = text(comment.id, "GitHub pull request comment " .. index .. " id"),
			body = text(comment.body, "GitHub pull request comment " .. index .. " body", true),
			url = text(comment.url, "GitHub pull request comment " .. index .. " URL"),
		}
	end
	return {
		number = value.number,
		title = value.title,
		body = value.body,
		url = value.url,
		state = value.state,
		review = review,
		checks = checks(value.statusCheckRollup),
		comments = comments,
	}
end

local function metadata(value)
	return "# "
		.. value.title
		.. "\n\n"
		.. (value.body ~= "" and value.body or "No pull request body.")
		.. "\n\nState: "
		.. value.state
		.. "\nReview decision: "
		.. value.review
end

local function entry(id, kind, ref, content, source, reason)
	return {
		id = id,
		kind = kind,
		ref = ref,
		content = content,
		provenance = { source = source, ref = ref },
		trust = "manual",
		token_estimate = {
			status = "unavailable",
			reason = "GitHub pull request content has not been provider-counted",
		},
		transfer = { eligible = false, reason = reason },
		policy_decision = "manual trust is required for imported GitHub content",
	}
end

function M.import(opts)
	if type(opts) ~= "table" then
		fail("import requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "number"
			and key ~= "task_id"
			and key ~= "pack_id"
			and key ~= "comment_ids"
			and key ~= "cwd"
			and key ~= "run"
			and key ~= "at"
		then
			fail("import contains unsupported field: " .. tostring(key))
		end
	end
	local pull_number = number(opts.number)
	local task_id = identifier(opts.task_id, "task_id")
	local pack_id = opts.pack_id and identifier(opts.pack_id, "pack_id") or "github-pr-" .. pull_number
	local comment_ids = selected(opts.comment_ids)
	if opts.cwd ~= nil and (type(opts.cwd) ~= "string" or opts.cwd == "") then
		fail("cwd must be a non-empty string")
	end
	if type(opts.run) ~= "function" then
		fail("run must be a function")
	end
	if opts.at ~= nil and (type(opts.at) ~= "number" or opts.at < 0 or opts.at % 1 ~= 0) then
		fail("at must be a non-negative integer timestamp")
	end
	local capability = gh.detect({ cwd = opts.cwd, run = opts.run })
	if not capability.available then
		fail(capability.reason)
	end
	if not capability.capabilities.pull_request_import.available then
		fail(capability.capabilities.pull_request_import.reason)
	end
	local view = invoke(opts.run, {
		"gh",
		"pr",
		"view",
		tostring(pull_number),
		"--json",
		"number,title,body,url,state,reviewDecision,statusCheckRollup,comments",
	}, opts.cwd, "GitHub pull request query")
	if view.code ~= 0 then
		fail("GitHub pull request query failed")
	end
	local ok, decoded = pcall(vim.json.decode, view.stdout)
	if not ok then
		fail("GitHub pull request query returned invalid JSON")
	end
	local pull_request = document(decoded)
	if pull_request.number ~= pull_number then
		fail("GitHub pull request query returned a different pull request")
	end
	local diff =
		invoke(opts.run, { "gh", "pr", "diff", tostring(pull_number), "--patch" }, opts.cwd, "GitHub pull request diff")
	if diff.code ~= 0 then
		fail("GitHub pull request diff failed")
	end
	local entries = {
		entry(
			"github-pr-" .. pull_request.number,
			"github_pull_request",
			pull_request.url,
			metadata(pull_request),
			"github-pull-request",
			"GitHub pull request context requires explicit trust confirmation"
		),
		entry(
			"github-pr-diff-" .. pull_request.number,
			"github_pull_request_diff",
			pull_request.url .. "#diff",
			diff.stdout ~= "" and diff.stdout or "No pull request diff.",
			"github-pull-request-diff",
			"GitHub pull request diff requires explicit trust confirmation"
		),
		entry(
			"github-pr-checks-" .. pull_request.number,
			"github_pull_request_checks",
			pull_request.url .. "#checks",
			"Status checks:\n" .. vim.json.encode(pull_request.checks),
			"github-pull-request-checks",
			"GitHub pull request checks require explicit trust confirmation"
		),
	}
	local evidence = {
		{ kind = "github-pull-request", ref = pull_request.url },
		{ kind = "github-pull-request-diff", ref = pull_request.url .. "#diff" },
		{ kind = "github-pull-request-checks", ref = pull_request.url .. "#checks" },
	}
	for index, comment in ipairs(pull_request.comments) do
		if comment_ids[comment.id] then
			comment_ids[comment.id] = nil
			table.insert(
				entries,
				entry(
					"github-pr-comment-" .. pull_request.number .. "-" .. index,
					"github_pull_request_comment",
					comment.url,
					comment.body ~= "" and comment.body or "No comment body.",
					"github-pull-request-comment",
					"GitHub pull request comment requires explicit trust confirmation"
				)
			)
			table.insert(evidence, { kind = "github-pull-request-comment", ref = comment.url })
		end
	end
	for id in pairs(comment_ids) do
		fail("selected GitHub pull request comment was not returned: " .. id)
	end
	return {
		task = task.new({
			id = task_id,
			objective = pull_request.title,
			evidence = evidence,
			created_at = opts.at,
			updated_at = opts.at,
		}),
		pack = pack.new({ id = pack_id, task_id = task_id, entries = entries }),
	}
end

return M
