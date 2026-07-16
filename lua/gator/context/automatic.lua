local pack = require("gator.context.pack")
local M = {}

local function fail(message)
	error("Gator automatic context: " .. message, 3)
end

function M.attach(opts)
	if type(opts) ~= "table" or not pack.is(opts.pack) or type(opts.policy) ~= "function" then
		fail("attach requires a context pack and policy callback")
	end
	if opts.audit ~= nil and type(opts.audit) ~= "function" then
		fail("audit must be a function")
	end
	local entries, audit = {}, {}
	for index, entry in ipairs(opts.pack.entries) do
		local ok, decision = pcall(opts.policy, entry)
		if
			not ok
			or type(decision) ~= "table"
			or type(decision.allowed) ~= "boolean"
			or type(decision.reason) ~= "string"
			or decision.reason == ""
		then
			fail("policy must return explicit allowed and reason fields")
		end
		local allowed = decision.allowed and entry.transfer.eligible
		audit[index] = {
			id = entry.id,
			allowed = allowed,
			reason = allowed and decision.reason
				or (entry.transfer.eligible and decision.reason or entry.transfer.reason),
		}
		if allowed then
			local selected = vim.deepcopy(entry)
			selected.policy_decision = decision.reason
			table.insert(entries, selected)
		end
	end
	local selected = pack.new({ id = opts.pack.id, task_id = opts.pack.task_id, entries = entries })
	if opts.audit then
		opts.audit(vim.deepcopy(audit))
	end
	return selected, audit
end

return M
