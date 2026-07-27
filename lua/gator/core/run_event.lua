local redact = require("gator.policy.redact")

local M = { schema_version = 1 }

M.types = {
	["run.created"] = true,
	["run.state"] = true,
	["run.recovered"] = true,
	["trust.applied"] = true,
	["provider.selected"] = true,
	["provider.session"] = true,
	["provider.running"] = true,
	["provider.settled"] = true,
	["provider.error"] = true,
	["provider.exited"] = true,
	["provider.usage"] = true,
	["approval.requested"] = true,
	["approval.decided"] = true,
	["context.prepared"] = true,
	["context.sent"] = true,
	["context.cancelled"] = true,
	["handoff.prepared"] = true,
	["review.recorded"] = true,
}

local forbidden = {
	text = true,
	body = true,
	content = true,
	prompt = true,
	diff = true,
	transcript = true,
	output = true,
}

local function fail(message)
	error("Gator run event: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function timestamp(value)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("at must be a non-negative integer timestamp")
	end
	return value
end

local function value(input, path)
	local kind = type(input)
	if kind == "string" then
		if #input > 2048 then
			fail(path .. " text exceeds 2048 bytes")
		end
		return redact.text(input)
	end
	if kind == "number" or kind == "boolean" then
		return input
	end
	if kind ~= "table" then
		fail(path .. " must be JSON-compatible")
	end
	local result = {}
	if vim.islist(input) then
		for index, item in ipairs(input) do
			result[index] = value(item, path .. "[" .. index .. "]")
		end
		return result
	end
	for key, item in pairs(input) do
		if type(key) ~= "string" then
			fail(path .. " keys must be strings")
		end
		if forbidden[key:lower()] then
			fail(path .. " must not retain raw " .. key)
		end
		result[key] = value(item, path .. "." .. key)
	end
	return redact.value(result)
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be an object")
	end
	for key in pairs(attrs) do
		if key ~= "schema_version" and key ~= "id" and key ~= "run_id" and key ~= "sequence" and key ~= "type" and key ~= "at" and key ~= "payload" then
			fail("attributes contain unsupported field: " .. tostring(key))
		end
	end
	if attrs.schema_version ~= M.schema_version then
		fail("schema_version is unsupported")
	end
	if type(attrs.sequence) ~= "number" or attrs.sequence < 0 or attrs.sequence % 1 ~= 0 then
		fail("sequence must be a non-negative integer")
	end
	if type(attrs.type) ~= "string" or not M.types[attrs.type] then
		fail("type is unavailable")
	end
	if attrs.payload ~= nil and (type(attrs.payload) ~= "table" or vim.islist(attrs.payload)) then
		fail("payload must be an object")
	end
	return {
		schema_version = M.schema_version,
		id = identifier(attrs.id, "id"),
		run_id = identifier(attrs.run_id, "run_id"),
		sequence = attrs.sequence,
		type = attrs.type,
		at = timestamp(attrs.at),
		payload = value(attrs.payload or {}, "payload"),
	}
end

function M.from_record(value)
	return M.new(value)
end

return M
