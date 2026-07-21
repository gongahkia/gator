local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")

local M = {}
local kinds = { created = true, modified = true, deleted = true }

local function fail(message)
	error("Gator file event: " .. redact.text(tostring(message)), 3)
end

local function path(value)
	if type(value) ~= "string" or value == "" or value:sub(1, 1) == "/" or value:find("..", 1, true) then
		fail("path must be a relative repository path")
	end
	return value
end

local function base(value, event_type, allowed)
	if type(value) ~= "table" then
		fail("attributes must be a table")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	return {
		schema_version = provider_event.schema_version,
		id = value.id,
		run_id = value.run_id,
		provider = value.provider,
		sequence = value.sequence,
		at = value.at,
		type = event_type,
	}
end

function M.change(value)
	local result = base(value, "file.change", {
		id = true,
		run_id = true,
		provider = true,
		sequence = true,
		at = true,
		path = true,
		kind = true,
		before = true,
		after = true,
	})
	if type(value.kind) ~= "string" or not kinds[value.kind] then
		fail("file change kind is unavailable")
	end
	result.payload = { path = path(value.path), kind = value.kind, before = value.before, after = value.after }
	return provider_event.new(result)
end

function M.diff(value)
	local result = base(value, "file.diff", {
		id = true,
		run_id = true,
		provider = true,
		sequence = true,
		at = true,
		patch = true,
		paths = true,
	})
	if type(value.patch) ~= "string" or type(value.paths) ~= "table" or not vim.islist(value.paths) then
		fail("diff requires patch text and path array")
	end
	local paths = {}
	for index, item in ipairs(value.paths) do
		paths[index] = path(item)
	end
	result.payload = { patch = value.patch, paths = paths }
	return provider_event.new(result)
end

return M
