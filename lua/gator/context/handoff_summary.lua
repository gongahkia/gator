local pack = require("gator.context.handoff_pack")
local evidence = require("gator.context.handoff_evidence")
local redact = require("gator.policy.redact")
local M = { schema_version = 1, defaults = { author = "user", max_chars = 4096 } }
local Summary = {}
local settings = vim.deepcopy(M.defaults)

Summary.__index = Summary

local function fail(message)
	error("Gator handoff summary: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function content(value)
	if type(value) ~= "string" or value == "" then
		fail("content must be non-empty text")
	end
	local result = redact.text(value)
	if #result > settings.max_chars then
		fail("content exceeds configured max_chars")
	end
	return result
end

local function authoring(value)
	if type(value) ~= "table" then
		fail("authoring settings must be a table")
	end
	for key in pairs(value) do
		if key ~= "author" and key ~= "max_chars" then
			fail("authoring settings contain unsupported field: " .. tostring(key))
		end
	end
	if value.author ~= "user" and value.author ~= "source" and value.author ~= "gator" then
		fail("authoring author must be user, source, or gator")
	end
	if type(value.max_chars) ~= "number" or value.max_chars < 1 or value.max_chars % 1 ~= 0 then
		fail("authoring max_chars must be a positive integer")
	end
	return { author = value.author, max_chars = value.max_chars }
end

function M.configure(value)
	settings = authoring(value)
	return vim.deepcopy(settings)
end

function M.settings()
	return vim.deepcopy(settings)
end

local function attrs(value)
	if type(value) ~= "table" then
		fail("attributes must be a table")
	end
	for key in pairs(value) do
		if
			key ~= "schema_version"
			and key ~= "id"
			and key ~= "pack_id"
			and key ~= "task_id"
			and key ~= "author"
			and key ~= "state"
			and key ~= "reason"
			and key ~= "content"
		then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	if value.schema_version ~= nil and value.schema_version ~= M.schema_version then
		fail("schema_version is unsupported")
	end
	if value.author ~= "user" and value.author ~= "gator" then
		fail("author must be user or gator")
	end
	local result = {
		id = identifier(value.id, "id"),
		pack_id = identifier(value.pack_id, "pack_id"),
		task_id = identifier(value.task_id, "task_id"),
		author = value.author,
		state = value.state or "ready",
	}
	if result.state == "ready" then
		if value.reason ~= nil then
			fail("ready summaries must not contain a reason")
		end
		result.content = content(value.content)
	elseif result.state == "unavailable" then
		if value.content ~= nil then
			fail("unavailable summaries must not contain content")
		end
		result.reason = content(value.reason)
	else
		fail("state must be ready or unavailable")
	end
	return result
end

function M.user(opts)
	if type(opts) ~= "table" then
		fail("user requires options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "pack" and key ~= "content" then
			fail("user contains unsupported field: " .. tostring(key))
		end
	end
	if not pack.is(opts.pack) then
		fail("user requires a canonical handoff pack")
	end
	local record = pack.to_record(opts.pack)
	return M.new({
		id = opts.id,
		pack_id = record.id,
		task_id = record.task_id,
		author = "user",
		content = opts.content,
	})
end

function M.new(value)
	return setmetatable(attrs(value), Summary)
end

local function synthesis(value, source)
	local lines = { "Handoff summary", "Source provider: " .. source.provider, "Context:" }
	for _, entry in ipairs(value.entries) do
		table.insert(lines, "- " .. entry.kind .. ": " .. redact.text(entry.ref))
	end
	if #source.evidence.decisions > 0 then
		table.insert(lines, "Decisions:")
		for _, item in ipairs(source.evidence.decisions) do
			table.insert(lines, "- " .. item.summary)
		end
	end
	if #source.evidence.outcomes > 0 then
		table.insert(lines, "Outcomes:")
		for _, item in ipairs(source.evidence.outcomes) do
			table.insert(lines, "- " .. item.summary)
		end
	end
	return redact.text(table.concat(lines, "\n"))
end

function M.synthesize(opts)
	if type(opts) ~= "table" then
		fail("synthesize requires options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "pack" and key ~= "evidence" then
			fail("synthesize contains unsupported field: " .. tostring(key))
		end
	end
	if not pack.is(opts.pack) or not evidence.is(opts.evidence) then
		fail("synthesize requires canonical handoff pack and evidence records")
	end
	local pack_record = pack.to_record(opts.pack)
	local evidence_record = evidence.to_record(opts.evidence)
	if pack_record.task_id ~= evidence_record.task_id then
		fail("handoff pack and evidence must belong to the same task")
	end
	if evidence_record.state ~= "ready" then
		return M.new({
			id = opts.id,
			pack_id = pack_record.id,
			task_id = pack_record.task_id,
			author = "gator",
			state = "unavailable",
			reason = evidence_record.reason,
		})
	end
	return M.new({
		id = opts.id,
		pack_id = pack_record.id,
		task_id = pack_record.task_id,
		author = "gator",
		content = synthesis(pack_record, { provider = evidence_record.source.provider, evidence = evidence_record }),
	})
end

function M.is(value)
	return getmetatable(value) == Summary
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.context.handoff_summary.new")
	end
	local canonical = attrs(value)
	return {
		schema_version = M.schema_version,
		id = canonical.id,
		pack_id = canonical.pack_id,
		task_id = canonical.task_id,
		author = canonical.author,
		state = canonical.state,
		content = canonical.content,
		reason = canonical.reason,
	}
end

function M.from_record(value)
	return M.new(value)
end

return M
