local filesystem = require("gator.core.filesystem")
local provider_event = require("gator.core.provider_event")
local redact = require("gator.policy.redact")

local M = { schema_version = 1 }
local Store = {}

Store.__index = Store

local function fail(message)
	error("Gator event cursor: " .. redact.text(tostring(message)), 3)
end

local function run_id(value)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail("run id must be a lowercase identifier")
	end
	return value
end

local function document(value)
	if
		type(value) ~= "table"
		or value.schema_version ~= M.schema_version
		or type(value.cursors) ~= "table"
		or not vim.islist(value.cursors)
	then
		fail("cursor document has an unsupported schema")
	end
	local seen = {}
	for _, value in ipairs(value.cursors) do
		if
			type(value) ~= "table"
			or type(value.sequence) ~= "number"
			or value.sequence < 0
			or value.sequence % 1 ~= 0
		then
			fail("cursor document has an invalid sequence")
		end
		value.run_id = run_id(value.run_id)
		if seen[value.run_id] then
			fail("cursor document has duplicate run ids")
		end
		seen[value.run_id] = true
	end
	return value
end

function M.open(path, opts)
	if type(path) ~= "string" or path == "" then
		fail("path must be non-empty")
	end
	opts = opts or {}
	if type(opts) ~= "table" or (opts.filesystem ~= nil and not filesystem.is(opts.filesystem)) then
		fail("open requires an optional filesystem boundary")
	end
	return setmetatable({ path = path, filesystem = opts.filesystem or filesystem.new() }, Store)
end

function M.is(value)
	return getmetatable(value) == Store
end

function Store:read()
	if not M.is(self) then
		fail("read requires a cursor store")
	end
	if not self.filesystem:readable(self.path) then
		return { schema_version = M.schema_version, cursors = {} }
	end
	local ok, value = pcall(vim.json.decode, self.filesystem:read(self.path))
	if not ok then
		fail("cursor document is not valid JSON")
	end
	return vim.deepcopy(document(value))
end

function Store:write(value)
	value = document(vim.deepcopy(value))
	local parent = vim.fn.fnamemodify(self.path, ":h")
	if not self.filesystem:mkdir(parent) then
		fail("cannot create cursor directory")
	end
	local temporary = self.path .. ".tmp-" .. vim.uv.hrtime()
	if not self.filesystem:write(temporary, vim.json.encode(value)) then
		self.filesystem:remove(temporary)
		fail("cannot write cursor document")
	end
	if not self.filesystem:rename(temporary, self.path) then
		self.filesystem:remove(temporary)
		fail("cannot replace cursor document")
	end
end

function Store:get(value)
	value = run_id(value)
	for _, cursor in ipairs(self:read().cursors) do
		if cursor.run_id == value then
			return cursor.sequence
		end
	end
	return -1
end

function Store:assert_next(value)
	if not provider_event.is(value) then
		fail("cursor requires a normalized provider event")
	end
	local expected = self:get(value.run_id) + 1
	if value.sequence ~= expected then
		fail("provider event sequence must be " .. expected .. " for run " .. value.run_id)
	end
	return expected
end

function Store:classify(value)
	if not provider_event.is(value) then
		fail("cursor requires a normalized provider event")
	end
	local sequence = self:get(value.run_id)
	if value.sequence <= sequence then
		return { status = "duplicate", sequence = sequence }
	end
	local expected = sequence + 1
	if value.sequence == expected then
		return { status = "next", sequence = sequence }
	end
	return { status = "gap", sequence = sequence, expected = expected }
end

function Store:advance(value)
	self:assert_next(value)
	local data = self:read()
	for _, cursor in ipairs(data.cursors) do
		if cursor.run_id == value.run_id then
			cursor.sequence = value.sequence
			self:write(data)
			return value.sequence
		end
	end
	table.insert(data.cursors, { run_id = value.run_id, sequence = value.sequence })
	table.sort(data.cursors, function(a, b)
		return a.run_id < b.run_id
	end)
	self:write(data)
	return value.sequence
end

return M
