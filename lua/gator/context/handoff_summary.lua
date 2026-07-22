local pack = require("gator.context.handoff_pack")
local redact = require("gator.policy.redact")
local M = { schema_version = 1 }
local Summary = {}

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
	return redact.text(value)
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
			and key ~= "content"
		then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	if value.schema_version ~= nil and value.schema_version ~= M.schema_version then
		fail("schema_version is unsupported")
	end
	if value.author ~= "user" then
		fail("author must be user")
	end
	return {
		id = identifier(value.id, "id"),
		pack_id = identifier(value.pack_id, "pack_id"),
		task_id = identifier(value.task_id, "task_id"),
		author = "user",
		content = content(value.content),
	}
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

function M.is(value)
	return getmetatable(value) == Summary
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.context.handoff_summary.user")
	end
	local canonical = attrs(value)
	return {
		schema_version = M.schema_version,
		id = canonical.id,
		pack_id = canonical.pack_id,
		task_id = canonical.task_id,
		author = canonical.author,
		content = canonical.content,
	}
end

function M.from_record(value)
	return M.new(value)
end

return M
