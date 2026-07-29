local completion = require("gator.completion")

local M = {}

function M.new()
	return setmetatable({}, { __index = M })
end

function M:enabled()
	local status = completion.status()
	return status.enabled and status.configured
end

function M:get_completions(ctx, callback)
	completion.request(ctx.bufnr, function(items)
		local values = {}
		for _, item in ipairs(items or {}) do
			local text = item.text or item.insert_text
			if type(text) == "string" and text ~= "" then
				table.insert(values, { label = text:match("^[^\n]*") or text, insertText = text })
			end
		end
		callback({ items = values, is_incomplete_forward = false, is_incomplete_backward = false })
	end)
end

return M
