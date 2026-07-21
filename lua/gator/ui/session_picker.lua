local capabilities = require("gator.adapters.capabilities")
local session = require("gator.core.session")
local picker = require("gator.ui.picker")
local M = {}

local function fail(message)
	error("Gator session picker: " .. message, 3)
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.resume) ~= "function" or type(opts.attach) ~= "function" then
		fail("open requires sessions, capabilities, resume, and attach callbacks")
	end
	for key in pairs(opts) do
		if
			key ~= "sessions"
			and key ~= "capabilities"
			and key ~= "resume"
			and key ~= "attach"
			and key ~= "on_cancel"
		then
			fail("open contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.sessions) ~= "table" or not vim.islist(opts.sessions) or type(opts.capabilities) ~= "table" then
		fail("sessions must be an array and capabilities must be a table")
	end
	local available, items = {}, {}
	for _, value in ipairs(opts.sessions) do
		if not session.is(value) then
			fail("sessions must contain provider-owned Gator sessions")
		end
		local contract = opts.capabilities[value.provider]
		if not capabilities.is(contract) or contract.provider ~= value.provider then
			fail("each session requires a matching capability contract")
		end
		if capabilities.supports(contract, "session", "resume") then
			local reference = session.reference(value)
			available[value.provider .. ":" .. value.id] = reference
			items[#items + 1] = { id = value.provider .. ":" .. value.id, label = value.provider .. " · " .. value.id }
		end
	end
	return picker.open({
		title = "Gator sessions",
		items = items,
		on_select = function(item)
			local reference = available[item.id]
			local resumed = opts.resume(vim.deepcopy(reference))
			if resumed == false then
				fail("native session resume failed")
			end
			opts.attach(vim.deepcopy(reference))
		end,
		on_cancel = opts.on_cancel,
	})
end

return M
