local M = {}
local highlight = require("gator.ui.highlight")
local glyphs = require("gator.ui.glyphs")
local actions = {
	next = true,
	previous = true,
	confirm = true,
	cancel = true,
	toggle = true,
	accept = true,
	reject = true,
	undo = true,
	prompt = true,
	handoff = true,
	journal = true,
	fork = true,
	context = true,
	review = true,
	parallel = true,
	runbook = true,
	stop = true,
	resume = true,
	close = true,
	help = true,
	grow = true,
	shrink = true,
	fullscreen = true,
	layout = true,
}
local settings = { keymaps = {}, screen_reader = true, icons = "unicode" }

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

function M.configure(opts)
	if type(opts) ~= "table" or type(opts.keymaps) ~= "table" or type(opts.screen_reader) ~= "boolean" then
		fail("settings require keymaps and screen_reader")
	end
	local icons = opts.icons or "unicode"
	glyphs.configure(icons)
	settings = { keymaps = vim.deepcopy(opts.keymaps), screen_reader = opts.screen_reader, icons = icons }
	return vim.deepcopy(settings)
end

function M.keymaps(defaults, handlers)
	if type(defaults) ~= "table" or type(handlers) ~= "table" then
		fail("keymaps require defaults and handlers")
	end
	local result = vim.deepcopy(defaults)
	for action, lhs in pairs(settings.keymaps) do
		if handlers[action] then
			result[action] = lhs
		end
	end
	return result
end

function M.panel(buffer, defaults, handlers)
	M.bind(buffer, M.keymaps(defaults, handlers), handlers)
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
	highlight.apply(buffer, lines)
	vim.bo[buffer].modifiable = false
	vim.bo[buffer].filetype = "gator-text"
end

function M.render(buffer, lines, filetype)
	if type(filetype) ~= "string" or filetype == "" then
		fail("render requires a non-empty filetype")
	end
	local rendered = glyphs.decorate(lines, filetype)
	if settings.screen_reader then
		M.text(buffer, rendered)
		return
	end
	vim.bo[buffer].modifiable = true
	vim.api.nvim_buf_set_lines(buffer, 0, -1, false, rendered)
	highlight.apply(buffer, rendered)
	vim.bo[buffer].modifiable = false
	vim.bo[buffer].filetype = filetype
end

return M
