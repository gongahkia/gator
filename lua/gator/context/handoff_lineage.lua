local lineage = require("gator.core.handoff_lineage")
local session = require("gator.core.session")
local evidence = require("gator.context.handoff_evidence")
local pack = require("gator.context.handoff_pack")
local redact = require("gator.policy.redact")
local M = {}

local function fail(message)
	error("Gator handoff lineage: " .. redact.text(tostring(message)), 3)
end

local function snapshot(value)
	if value == nil then
		return nil
	end
	if type(value) ~= "table" then
		fail("snapshot must be a workspace snapshot record")
	end
	return { id = value.id, run_id = value.run_id, type = value.type, at = value.at }
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "id"
			and key ~= "pack"
			and key ~= "evidence"
			and key ~= "target"
			and key ~= "source_snapshot"
			and key ~= "target_snapshot"
			and key ~= "at"
		then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if not pack.is(opts.pack) or not evidence.is(opts.evidence) or not session.is(opts.target) then
		fail("new requires canonical handoff pack, evidence, and target session")
	end
	local context = pack.to_record(opts.pack)
	local source = evidence.to_record(opts.evidence)
	if source.task_id ~= context.task_id or opts.target.task_id ~= context.task_id then
		fail("pack, evidence, and target session must belong to the same task")
	end
	if source.state ~= "ready" or not source.source.session then
		return {
			available = false,
			task_id = context.task_id,
			pack_id = context.id,
			reason = redact.text(source.reason or "source evidence has no provider-native session"),
		}
	end
	return lineage.new({
		id = opts.id,
		task_id = context.task_id,
		pack_id = context.id,
		source = source.source,
		target = session.reference(opts.target),
		snapshots = { source = snapshot(opts.source_snapshot), target = snapshot(opts.target_snapshot) },
		at = opts.at,
	})
end

function M.is(value)
	return lineage.is(value)
end

function M.to_record(value)
	return lineage.to_record(value)
end

function M.from_record(value)
	return lineage.from_record(value)
end

function M.persist(opts)
	if type(opts) ~= "table" then
		fail("persist requires options")
	end
	for key in pairs(opts) do
		if key ~= "lineage" and key ~= "backend" then
			fail("persist contains unsupported field: " .. tostring(key))
		end
	end
	if
		not lineage.is(opts.lineage)
		or type(opts.backend) ~= "table"
		or type(opts.backend.append_handoff_lineage) ~= "function"
	then
		fail("persist requires canonical lineage and storage backend")
	end
	return opts.backend:append_handoff_lineage(opts.lineage)
end

return M
