local completion = require("gator.completion")

local Source = {}
Source.__index = Source

function Source:new()
	return setmetatable({}, Source)
end

function Source:is_available()
	return completion.status().enabled and completion.status().configured
end

function Source:get_position_encoding_kind()
	return "utf-8"
end

function Source:complete(params, callback)
	local _, ticket = completion.request(params.context.bufnr, function(items)
		local values = {}
		for _, item in ipairs(items or {}) do
			table.insert(values, {
				label = item.text or item.insert_text,
				insertText = item.text or item.insert_text,
				kind = 1,
				data = { gator_completion_id = item.id, gator_completion_root = ticket and ticket.root or nil },
			})
		end
		callback(values)
	end)
end

return Source
