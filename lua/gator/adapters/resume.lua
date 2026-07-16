local capabilities = require("gator.adapters.capabilities")
local session = require("gator.core.session")
local metadata = require("gator.core.session_metadata")
local M = {}

local function fail(message)
	error("Gator adapter resume: " .. message, 3)
end

local function matching(value, record)
	return value.task_id == record.task_id
		and value.provider == record.provider
		and value.id == record.id
		and value.owner == record.owner
end

function M.session(opts)
	if type(opts) ~= "table" or not session.is(opts.session) or not capabilities.is(opts.capabilities) then
		fail("resume requires a Gator session and capability contract")
	end
	for key in pairs(opts) do
		if
			key ~= "session"
			and key ~= "capabilities"
			and key ~= "native_resume"
			and key ~= "metadata"
			and key ~= "persist_orphan"
		then
			fail("resume contains unsupported field: " .. tostring(key))
		end
	end
	if opts.capabilities.provider ~= opts.session.provider then
		fail("capability provider must match the session provider")
	end
	local resumable = capabilities.supports(opts.capabilities, "session", "resume")
	if resumable then
		if type(opts.native_resume) ~= "function" then
			fail("resumable sessions require a native_resume callback")
		end
		opts.native_resume(session.reference(opts.session))
		return { status = "resumed", session = session.reference(opts.session) }
	end
	if
		not metadata.is(opts.metadata)
		or not matching(opts.metadata, opts.session)
		or type(opts.persist_orphan) ~= "function"
	then
		fail("non-resumable sessions require matching metadata and persist_orphan callback")
	end
	local record = metadata.to_record(opts.metadata)
	opts.persist_orphan(record)
	return { status = "orphaned", reason = opts.capabilities.session.reason, metadata = record }
end

return M
