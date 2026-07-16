local errors = require("gator.error")
local M = {}
local Pack = {}

Pack.__index = Pack

M.trust = { provenance = true, repository = true, manual = true }

local function fail(detail)
	errors.raise(errors.new("context_pack.invalid", "Context pack is invalid", {
		detail = detail,
		remedy = "Provide ordered context references with explicit provenance, trust, estimate, and transfer fields.",
	}))
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function require_identifier(value, name)
	value = require_string(value, name)
	if not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function validate_fields(value, allowed, name)
	for key in pairs(value) do
		if not allowed[key] then
			fail(name .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function provenance(value)
	if type(value) ~= "table" then
		fail("provenance must be a table")
	end
	validate_fields(value, { source = true, ref = true }, "provenance")
	return {
		source = require_string(value.source, "provenance.source"),
		ref = require_string(value.ref, "provenance.ref"),
	}
end

local function estimate(value)
	if type(value) ~= "table" then
		fail("token_estimate must be a table")
	end
	if value.status == "estimated" then
		validate_fields(value, { status = true, tokens = true }, "estimated token estimate")
		if type(value.tokens) ~= "number" or value.tokens < 0 or value.tokens % 1 ~= 0 then
			fail("estimated token count must be a non-negative integer")
		end
		return { status = "estimated", tokens = value.tokens }
	end
	if value.status == "unavailable" then
		validate_fields(value, { status = true, reason = true }, "unavailable token estimate")
		return { status = "unavailable", reason = require_string(value.reason, "token estimate reason") }
	end
	fail("token_estimate.status must be estimated or unavailable")
end

local function transfer(value)
	if type(value) ~= "table" or type(value.eligible) ~= "boolean" then
		fail("transfer must declare boolean eligibility")
	end
	if value.eligible then
		validate_fields(value, { eligible = true }, "eligible transfer")
		return { eligible = true }
	end
	validate_fields(value, { eligible = true, reason = true }, "ineligible transfer")
	return { eligible = false, reason = require_string(value.reason, "transfer reason") }
end

function M.entry(attrs)
	if type(attrs) ~= "table" then
		fail("entry attributes must be a table")
	end
	validate_fields(attrs, {
		id = true,
		kind = true,
		ref = true,
		provenance = true,
		trust = true,
		token_estimate = true,
		transfer = true,
		annotation = true,
		pinned = true,
		revision = true,
		retrieval_source = true,
		policy_decision = true,
	}, "entry")
	local trust = require_string(attrs.trust, "entry.trust")
	if not M.trust[trust] then
		fail("entry.trust is unknown: " .. trust)
	end
	local annotation = attrs.annotation
	if annotation ~= nil then
		annotation = require_string(annotation, "entry.annotation")
	end
	if attrs.pinned ~= nil and type(attrs.pinned) ~= "boolean" then
		fail("entry.pinned must be a boolean")
	end
	for _, key in ipairs({ "revision", "retrieval_source", "policy_decision" }) do
		if attrs[key] ~= nil and (type(attrs[key]) ~= "string" or attrs[key] == "") then
			fail("entry." .. key .. " must be a non-empty string")
		end
	end
	return {
		id = require_identifier(attrs.id, "entry.id"),
		kind = require_string(attrs.kind, "entry.kind"),
		ref = require_string(attrs.ref, "entry.ref"),
		provenance = provenance(attrs.provenance),
		trust = trust,
		token_estimate = estimate(attrs.token_estimate),
		transfer = transfer(attrs.transfer),
		annotation = annotation,
		pinned = attrs.pinned or false,
		revision = attrs.revision,
		retrieval_source = attrs.retrieval_source,
		policy_decision = attrs.policy_decision,
	}
end

local function entries(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("entries must be an array")
	end
	local result = {}
	for index, entry in ipairs(value) do
		result[index] = M.entry(entry)
	end
	return result
end

function M.new(attrs)
	if type(attrs) ~= "table" then
		fail("attributes must be a table")
	end
	validate_fields(attrs, { id = true, task_id = true, entries = true }, "pack")
	return setmetatable({
		id = require_identifier(attrs.id, "id"),
		task_id = require_identifier(attrs.task_id, "task_id"),
		entries = entries(attrs.entries),
	}, Pack)
end

function M.is(value)
	return getmetatable(value) == Pack
end

function M.to_record(value)
	if not M.is(value) then
		fail("value must be created by gator.context.pack.new")
	end
	local pack = M.new(value)
	return { id = pack.id, task_id = pack.task_id, entries = entries(pack.entries) }
end

function M.from_record(record)
	return M.new(record)
end

return M
