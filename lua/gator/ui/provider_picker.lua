local capabilities = require("gator.adapters.capabilities")
local picker = require("gator.ui.picker")
local M = {}

local function fail(message)
	error("Gator provider picker: " .. message, 3)
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.on_launch) ~= "function" then
		fail("open requires providers and an on_launch callback")
	end
	for key in pairs(opts) do
		if key ~= "providers" and key ~= "on_launch" and key ~= "on_cancel" then
			fail("open contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.providers) ~= "table" or not vim.islist(opts.providers) then
		fail("providers must be an array")
	end
	local available, items = {}, {}
	for _, provider in ipairs(opts.providers) do
		if not capabilities.is(provider) then
			fail("providers must contain capability contracts")
		end
		local native_transport = capabilities.supports(provider, "transport", "native")
		local managed_transport = capabilities.supports(provider, "transport", "managed")
		local native_auth = capabilities.supports(provider, "auth", "native")
		local user_confirmed = capabilities.supports(provider, "auth", "user_confirmed")
		if (native_transport or managed_transport) and (native_auth or user_confirmed) then
			available[provider.provider] = provider
			items[#items + 1] = {
				id = provider.provider,
				label = provider.provider
					.. (user_confirmed and " · user-confirmed; credentials not verified; ready" or " · ready")
					.. (managed_transport and " in Gator" or " to launch"),
			}
		end
	end
	table.sort(items, function(left, right)
		return left.id < right.id
	end)
	return picker.open({
		title = "Gator providers",
		items = items,
		on_select = function(item)
			opts.on_launch({ provider = item.id, capabilities = capabilities.to_record(available[item.id]) })
		end,
		on_cancel = opts.on_cancel,
	})
end

return M
