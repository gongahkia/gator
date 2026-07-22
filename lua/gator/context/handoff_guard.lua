local core_lineage = require("gator.core.handoff_lineage")
local evidence = require("gator.context.handoff_evidence")
local pack = require("gator.context.handoff_pack")
local redact = require("gator.policy.redact")
local M = {}
local Guard = {}
local Reservation = {}

Guard.__index = Guard
Reservation.__index = Reservation

local function fail(message)
	error("Gator handoff guard: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function same_source(left, right)
	return left.provider == right.provider and left.session.id == right.session.id
end

local function values(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("lineages must be an ordered array")
	end
	local result = {}
	for index, item in ipairs(value) do
		if not core_lineage.is(item) then
			fail("lineages[" .. index .. "] must be canonical")
		end
		result[index] = core_lineage.to_record(item)
	end
	return result
end

local function path(edges, from, target, seen)
	if from == target then
		return true
	end
	seen = seen or {}
	if seen[from] then
		return false
	end
	seen[from] = true
	for _, edge in ipairs(edges) do
		if edge.source.provider == from and path(edges, edge.target.provider, target, seen) then
			return true
		end
	end
	return false
end

local function request(value)
	if type(value) ~= "table" or not pack.is(value.pack) or not evidence.is(value.evidence) then
		fail("reservation requires canonical handoff pack and evidence")
	end
	local source_pack = pack.to_record(value.pack)
	local source = evidence.to_record(value.evidence)
	if source.task_id ~= source_pack.task_id then
		fail("handoff pack and evidence must belong to the same task")
	end
	if source.state ~= "ready" or not source.source.session then
		return nil,
			{
				available = false,
				task_id = source_pack.task_id,
				reason = source.reason or "source evidence has no provider-native session",
			}
	end
	local target_provider = identifier(value.target_provider, "target_provider")
	return {
		task_id = source_pack.task_id,
		pack_id = source_pack.id,
		source = source.source,
		target_provider = target_provider,
		target = { provider = target_provider },
	}
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "lineages" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	return setmetatable({ lineages = values(opts.lineages or {}), reservations = {} }, Guard)
end

function M.is(value)
	return getmetatable(value) == Guard
end

function M.is_reservation(value)
	return getmetatable(value) == Reservation
end

function Guard:reserve(opts)
	if not M.is(self) then
		fail("reserve requires a handoff guard")
	end
	local value, unavailable = request(opts)
	if not value then
		unavailable.reason = redact.text(unavailable.reason)
		return unavailable
	end
	local edges = vim.deepcopy(self.lineages)
	for _, reservation in pairs(self.reservations) do
		if reservation.state == "pending" then
			table.insert(edges, reservation.value)
		end
	end
	for _, edge in ipairs(edges) do
		if
			edge.task_id == value.task_id
			and edge.pack_id == value.pack_id
			and same_source(edge.source, value.source)
			and edge.target.provider == value.target_provider
		then
			return {
				available = false,
				task_id = value.task_id,
				reason = "duplicate handoff launch is already recorded or pending",
			}
		end
	end
	if path(edges, value.target_provider, value.source.provider) then
		return { available = false, task_id = value.task_id, reason = "handoff would create a provider cycle" }
	end
	local id = vim.fn.sha256(
		table.concat(
			{ value.task_id, value.pack_id, value.source.provider, value.source.session.id, value.target_provider },
			"\0"
		)
	)
	local reservation = setmetatable({ guard = self, id = id, value = value, state = "pending" }, Reservation)
	self.reservations[id] = reservation
	return reservation
end

function Reservation:status()
	if not M.is_reservation(self) then
		fail("status requires a handoff reservation")
	end
	return vim.deepcopy({ id = self.id, task_id = self.value.task_id, pack_id = self.value.pack_id, state = self.state })
end

function Reservation:cancel()
	if not M.is_reservation(self) or self.state ~= "pending" then
		return false
	end
	self.state = "cancelled"
	return true
end

function Reservation:commit(lineage)
	if not M.is_reservation(self) or self.state ~= "pending" or not core_lineage.is(lineage) then
		return false
	end
	local record = core_lineage.to_record(lineage)
	if
		record.task_id ~= self.value.task_id
		or record.pack_id ~= self.value.pack_id
		or not same_source(record.source, self.value.source)
		or record.target.provider ~= self.value.target_provider
	then
		fail("lineage does not match the reserved handoff")
	end
	table.insert(self.guard.lineages, record)
	self.state = "completed"
	return true
end

return M
