local gh = require("gator.github.gh")
local pack = require("gator.context.pack")
local task = require("gator.core.task")
local M = {}

local function fail(message)
	error("Gator GitHub issue import: " .. message, 3)
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

local function invoke(run, argv, cwd)
	local ok, result = pcall(run, argv, cwd)
	if not ok or type(result) ~= "table" or type(result.code) ~= "number" or result.code % 1 ~= 0 then
		fail("GitHub issue query failed")
	end
	if result.stdout ~= nil and type(result.stdout) ~= "string" then
		fail("GitHub issue query returned invalid stdout")
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

local function document(value)
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail("GitHub issue response must be an object")
	end
	for key in pairs(value) do
		if
			key ~= "number"
			and key ~= "title"
			and key ~= "body"
			and key ~= "labels"
			and key ~= "comments"
			and key ~= "url"
		then
			fail("GitHub issue response contains unsupported field: " .. tostring(key))
		end
	end
	number(value.number)
	text(value.title, "GitHub issue title")
	text(value.body, "GitHub issue body", true)
	text(value.url, "GitHub issue URL")
	if type(value.labels) ~= "table" or not vim.islist(value.labels) then
		fail("GitHub issue labels must be a list")
	end
	local labels = {}
	for index, label in ipairs(value.labels) do
		if type(label) ~= "table" or type(label.name) ~= "string" or label.name == "" then
			fail("GitHub issue label " .. index .. " must have a name")
		end
		labels[index] = label.name
	end
	if type(value.comments) ~= "table" or not vim.islist(value.comments) then
		fail("GitHub issue comments must be a list")
	end
	local comments = {}
	for index, comment in ipairs(value.comments) do
		if type(comment) ~= "table" then
			fail("GitHub issue comment " .. index .. " must be an object")
		end
		comments[index] = {
			id = text(comment.id, "GitHub issue comment " .. index .. " id"),
			body = text(comment.body, "GitHub issue comment " .. index .. " body", true),
			url = text(comment.url, "GitHub issue comment " .. index .. " URL"),
		}
	end
	return {
		number = value.number,
		title = value.title,
		body = value.body,
		url = value.url,
		labels = labels,
		comments = comments,
	}
end

local function content(title, body, labels)
	local result = "# " .. title .. "\n\n" .. (body ~= "" and body or "No issue body.")
	if #labels > 0 then
		result = result .. "\n\nLabels: " .. table.concat(labels, ", ")
	end
	return result
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
	local issue_number = number(opts.number)
	local task_id = identifier(opts.task_id, "task_id")
	local pack_id = opts.pack_id and identifier(opts.pack_id, "pack_id") or "github-issue-" .. issue_number
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
	if not capability.capabilities.issue_import.available then
		fail(capability.capabilities.issue_import.reason)
	end
	local result = invoke(
		opts.run,
		{ "gh", "issue", "view", tostring(issue_number), "--json", "number,title,body,labels,comments,url" },
		opts.cwd
	)
	if result.code ~= 0 then
		fail("GitHub issue query failed")
	end
	local ok, decoded = pcall(vim.json.decode, result.stdout)
	if not ok then
		fail("GitHub issue query returned invalid JSON")
	end
	local issue = document(decoded)
	if issue.number ~= issue_number then
		fail("GitHub issue query returned a different issue")
	end
	local entries = {
		{
			id = "github-issue-" .. issue.number,
			kind = "github_issue",
			ref = issue.url,
			content = content(issue.title, issue.body, issue.labels),
			provenance = { source = "github-issue", ref = issue.url },
			trust = "manual",
			token_estimate = { status = "unavailable", reason = "GitHub issue content has not been provider-counted" },
			transfer = { eligible = false, reason = "GitHub issue content requires explicit trust confirmation" },
			policy_decision = "manual trust is required for imported GitHub content",
		},
	}
	local evidence = { { kind = "github-issue", ref = issue.url } }
	for index, comment in ipairs(issue.comments) do
		if comment_ids[comment.id] then
			comment_ids[comment.id] = nil
			table.insert(entries, {
				id = "github-comment-" .. issue.number .. "-" .. index,
				kind = "github_comment",
				ref = comment.url,
				content = comment.body ~= "" and comment.body or "No comment body.",
				provenance = { source = "github-comment", ref = comment.url },
				trust = "manual",
				token_estimate = {
					status = "unavailable",
					reason = "GitHub comment content has not been provider-counted",
				},
				transfer = { eligible = false, reason = "GitHub comment content requires explicit trust confirmation" },
				policy_decision = "manual trust is required for imported GitHub content",
			})
			table.insert(evidence, { kind = "github-comment", ref = comment.url })
		end
	end
	for id in pairs(comment_ids) do
		fail("selected GitHub comment was not returned: " .. id)
	end
	return {
		task = task.new({
			id = task_id,
			objective = issue.title,
			evidence = evidence,
			created_at = opts.at,
			updated_at = opts.at,
		}),
		pack = pack.new({ id = pack_id, task_id = task_id, entries = entries }),
	}
end

return M
