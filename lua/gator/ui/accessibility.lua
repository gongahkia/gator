local M = {}
local actions = { next = true, previous = true, confirm = true, cancel = true }

local function fail(message)
	error("Gator accessibility: " .. message, 3)
end

function M.bind(buffer, keymaps, handlers)
	if type(buffer) ~= "number" or not vim.api.nvim_buf_is_valid(buffer) then
		fail("buffer must be valid")
	end
	if type(keymaps) ~= "table" or type(handlers) ~= "table" then
		fail("keymaps and handlers must be tables")
	end
	for action, lhs in pairs(keymaps) do
		if not actions[action] or type(lhs) ~= "string" or lhs == "" then
			fail("keymaps must use supported non-empty action bindings")
		end
		if type(handlers[action]) ~= "function" then
			fail("mapped action must have a handler: " .. action)
		end
		vim.keymap.set("n", lhs, handlers[action], { buffer = buffer, desc = "Gator " .. action, silent = true })
	end
end

function M.text(buffer, lines)
	if
		type(buffer) ~= "number"
		or not vim.api.nvim_buf_is_valid(buffer)
		or type(lines) ~= "table"
		or not vim.islist(lines)
	then
		fail("text requires a valid buffer and line array")
	end
	for index, line in ipairs(lines) do
		if type(line) ~= "string" then
			fail("text line " .. index .. " must be a string")
		end
	end
	vim.bo[buffer].modifiable = true
	vim.api.nvim_buf_set_lines(buffer, 0, -1, false, lines)
	vim.bo[buffer].modifiable = false
	vim.bo[buffer].filetype = "gator-text"
end

return M
