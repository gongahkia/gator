local pack = require("gator.context.handoff_pack")
local context_pack = require("gator.context.pack")
local redact = require("gator.policy.redact")
local M = { defaults = { review = "required" } }
local Review = {}
local settings = vim.deepcopy(M.defaults)

Review.__index = Review

local function fail(message)
	error("Gator handoff pack review: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function annotation(value)
	if type(value) ~= "string" or value == "" then
		fail("annotation must be non-empty text")
	end
	return redact.text(value)
end

local function index(entries, id)
	for position, entry in ipairs(entries) do
		if entry.id == id then
			return position
		end
	end
	return nil
end

local function require_ready(value)
	if value.state ~= "ready" then
		fail("review is " .. value.state)
	end
end

function M.configure(value)
	if type(value) ~= "table" or (value.review ~= "required" and value.review ~= "optional") then
		fail("review settings must declare required or optional enforcement")
	end
	settings = { review = value.review }
	return vim.deepcopy(settings)
end

function M.settings()
	return vim.deepcopy(settings)
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "pack" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if not pack.is(opts.pack) then
		fail("new requires a canonical handoff pack")
	end
	local value = pack.to_record(opts.pack)
	return setmetatable({
		id = value.id,
		task_id = value.task_id,
		entries = value.entries,
		state = "ready",
		mode = settings.review,
	}, Review)
end

function M.is(value)
	return getmetatable(value) == Review
end

function Review:status()
	if not M.is(self) then
		fail("status requires a handoff pack review")
	end
	return vim.deepcopy({
		id = self.id,
		task_id = self.task_id,
		state = self.state,
		mode = self.mode,
		approved = self.approved or false,
		entries = self.entries,
	})
end

function Review:append(entry)
	if not M.is(self) then
		fail("append requires a handoff pack review")
	end
	require_ready(self)
	entry = context_pack.entry(entry)
	if index(self.entries, entry.id) then
		fail("entry is already present: " .. entry.id)
	end
	table.insert(self.entries, entry)
	return self:status()
end

function Review:remove(id)
	if not M.is(self) then
		fail("remove requires a handoff pack review")
	end
	require_ready(self)
	id = identifier(id, "entry id")
	local position = index(self.entries, id)
	if not position then
		fail("entry is not present: " .. id)
	end
	local removed = table.remove(self.entries, position)
	return vim.deepcopy(removed)
end

function Review:move(id, destination)
	if not M.is(self) then
		fail("move requires a handoff pack review")
	end
	require_ready(self)
	id = identifier(id, "entry id")
	if type(destination) ~= "number" or destination % 1 ~= 0 or destination < 1 or destination > #self.entries then
		fail("destination must identify an available entry position")
	end
	local position = index(self.entries, id)
	if not position then
		fail("entry is not present: " .. id)
	end
	local entry = table.remove(self.entries, position)
	table.insert(self.entries, destination, entry)
	return self:status()
end

function Review:annotate(id, value)
	if not M.is(self) then
		fail("annotate requires a handoff pack review")
	end
	require_ready(self)
	id = identifier(id, "entry id")
	local position = index(self.entries, id)
	if not position then
		fail("entry is not present: " .. id)
	end
	self.entries[position].annotation = annotation(value)
	return self:status()
end

function Review:approve()
	if not M.is(self) then
		fail("approve requires a handoff pack review")
	end
	require_ready(self)
	self.approved = true
	return self:status()
end

function Review:commit()
	if not M.is(self) then
		fail("commit requires a handoff pack review")
	end
	if self.state ~= "ready" then
		return nil
	end
	if self.mode == "required" and not self.approved then
		fail("review approval is required before commit")
	end
	self.state = "completed"
	return pack.new({ id = self.id, task_id = self.task_id, entries = self.entries })
end

function Review:cancel()
	if not M.is(self) then
		fail("cancel requires a handoff pack review")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state = "cancelled"
	return true
end

return M
