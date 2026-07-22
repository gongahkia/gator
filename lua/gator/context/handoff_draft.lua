local capabilities = require("gator.adapters.capabilities")
local evidence = require("gator.context.handoff_evidence")
local pack = require("gator.context.handoff_pack")
local redact = require("gator.policy.redact")
local M =
	{ states = { ready = true, pending = true, completed = true, unavailable = true, failed = true, cancelled = true } }
local Request = {}

Request.__index = Request

local function fail(message)
	error("Gator handoff draft: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function reason(value, fallback)
	if value == nil then
		return fallback
	end
	if type(value) ~= "string" or value == "" then
		return fallback
	end
	return redact.text(value)
end

local function entry(value)
	return redact.value({
		id = value.id,
		kind = value.kind,
		ref = value.ref,
		provenance = value.provenance,
		trust = value.trust,
		transfer = value.transfer,
		annotation = value.annotation,
		pinned = value.pinned,
		revision = value.revision,
		retrieval_source = value.retrieval_source,
		policy_decision = value.policy_decision,
	})
end

local function payload(id, value, source)
	local entries = {}
	for index, item in ipairs(value.entries) do
		entries[index] = entry(item)
	end
	return redact.value({
		id = id,
		kind = "gator.handoff_summary_draft",
		task_id = value.task_id,
		source_session = source.session,
		context = { pack_id = value.id, entries = entries },
		evidence = { decisions = source.evidence.decisions, outcomes = source.evidence.outcomes },
		instruction = "Draft a concise handoff summary from these context references and source evidence. Do not include credentials or provider-native session data.",
	})
end

local function unavailable(id, task_id, detail)
	return setmetatable({ id = id, task_id = task_id, state = "unavailable", reason = redact.text(detail) }, Request)
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "pack" and key ~= "evidence" and key ~= "capabilities" and key ~= "send" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if not pack.is(opts.pack) or not evidence.is(opts.evidence) then
		fail("new requires canonical handoff pack and evidence records")
	end
	if not capabilities.is(opts.capabilities) or type(opts.send) ~= "function" then
		fail("new requires a capability contract and send callback")
	end
	local id = identifier(opts.id, "id")
	local pack_record = pack.to_record(opts.pack)
	local evidence_record = evidence.to_record(opts.evidence)
	if pack_record.task_id ~= evidence_record.task_id then
		fail("handoff pack and evidence must belong to the same task")
	end
	if evidence_record.state ~= "ready" then
		return unavailable(id, pack_record.task_id, evidence_record.reason)
	end
	local source = evidence_record.source
	if not source.session then
		return unavailable(id, pack_record.task_id, "source evidence has no provider-native session")
	end
	if opts.capabilities.provider ~= source.provider then
		fail("capability provider must match the source evidence provider")
	end
	local supported, detail = capabilities.supports(opts.capabilities, "session", "resume")
	if not supported then
		return unavailable(id, pack_record.task_id, "source session resume is unavailable: " .. detail)
	end
	return setmetatable({
		id = id,
		task_id = pack_record.task_id,
		state = "ready",
		source = source,
		request = payload(id, pack_record, { session = source.session, evidence = evidence_record }),
		send = opts.send,
	}, Request)
end

function M.is(value)
	return getmetatable(value) == Request
end

function Request:status()
	if not M.is(self) then
		fail("status requires a handoff draft request")
	end
	return vim.deepcopy({
		id = self.id,
		task_id = self.task_id,
		state = self.state,
		source = self.source,
		content = self.content,
		reason = self.reason,
	})
end

function Request:payload()
	if not M.is(self) then
		fail("payload requires a handoff draft request")
	end
	if self.state ~= "ready" and self.state ~= "pending" then
		return nil
	end
	return vim.deepcopy(self.request)
end

function Request:complete(content, detail)
	if not M.is(self) then
		fail("complete requires a handoff draft request")
	end
	if self.state ~= "pending" then
		return false
	end
	if detail ~= nil then
		self.state = "failed"
		self.reason = reason(detail, "source provider failed to draft a handoff summary")
		return false
	end
	if type(content) ~= "string" or content == "" then
		self.state = "failed"
		self.reason = "source provider returned no handoff summary"
		return false
	end
	self.state = "completed"
	self.content = redact.text(content)
	return true
end

function Request:dispatch()
	if not M.is(self) then
		fail("dispatch requires a handoff draft request")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state = "pending"
	local ok, accepted, detail = pcall(self.send, vim.deepcopy(self.request), function(content, failure)
		self:complete(content, failure)
	end)
	if not ok then
		if self.state == "pending" then
			self.state = "failed"
			self.reason = reason(accepted, "source provider failed to send a handoff draft request")
		end
		return false
	end
	if accepted == false and self.state == "pending" then
		self.state = "failed"
		self.reason = reason(detail, "source provider rejected the handoff draft request")
		return false
	end
	return true
end

function Request:cancel(detail)
	if not M.is(self) then
		fail("cancel requires a handoff draft request")
	end
	if self.state ~= "ready" and self.state ~= "pending" then
		return false
	end
	self.state = "cancelled"
	self.reason = reason(detail, "source handoff draft request was cancelled")
	return true
end

return M
