local M = { schema_version = 1 }
local Lineage = {}

Lineage.__index = Lineage

local function fail(message)
	error("Gator handoff lineage: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function opaque(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty opaque id")
	end
	return value
end

local function fields(value, allowed, name)
	if type(value) ~= "table" then
		fail(name .. " must be a table")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function reference(value, name)
	fields(value, { provider = true, id = true, owner = true }, name)
	if value.owner ~= "provider" then
		fail(name .. " owner must be provider")
	end
	return {
		provider = identifier(value.provider, name .. " provider"),
		id = opaque(value.id, name .. " id"),
		owner = "provider",
	}
end

local function source(value)
	fields(value, { provider = true, run_id = true, session = true }, "source")
	local session = reference(value.session, "source session")
	local provider = identifier(value.provider, "source provider")
	if session.provider ~= provider then
		fail("source session provider must match source provider")
	end
	return { provider = provider, run_id = identifier(value.run_id, "source run_id"), session = session }
end

local function snapshot(value, name)
	if value == nil then
		return nil
	end
	fields(value, { id = true, run_id = true, type = true, at = true }, name)
	if type(value.type) ~= "string" or not value.type:match("^[a-z][a-z0-9_.-]*$") then
		fail(name .. " type must be a lowercase identifier")
	end
	if type(value.at) ~= "number" or value.at < 0 or value.at % 1 ~= 0 then
		fail(name .. " at must be a non-negative integer timestamp")
	end
	return {
		id = identifier(value.id, name .. " id"),
		run_id = identifier(value.run_id, name .. " run_id"),
		type = value.type,
		at = value.at,
	}
end

local function attrs(value)
	fields(value, {
		schema_version = true,
		id = true,
		task_id = true,
		pack_id = true,
		source = true,
		target = true,
		snapshots = true,
		at = true,
	}, "attributes")
	if value.schema_version ~= nil and value.schema_version ~= M.schema_version then
		fail("schema_version is unsupported")
	end
	fields(value.snapshots, { source = true, target = true }, "snapshots")
	if type(value.at) ~= "number" or value.at < 0 or value.at % 1 ~= 0 then
		fail("at must be a non-negative integer timestamp")
	end
	return {
		id = identifier(value.id, "id"),
		task_id = identifier(value.task_id, "task_id"),
		pack_id = identifier(value.pack_id, "pack_id"),
		source = source(value.source),
		target = reference(value.target, "target"),
		snapshots = {
			source = snapshot(value.snapshots.source, "source snapshot"),
			target = snapshot(value.snapshots.target, "target snapshot"),
		},
		at = value.at,
	}
end

function M.new(value)
	return setmetatable(attrs(value), Lineage)
end

function M.is(value)
	return getmetatable(value) == Lineage
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.core.handoff_lineage.new")
	end
	local canonical = attrs(value)
	canonical.schema_version = M.schema_version
	return canonical
end

function M.from_record(value)
	return M.new(value)
end

return M
