local M = {}
local namespace = vim.api.nvim_create_namespace("GatorTextHighlights")
local configured = false

local groups = {
	GatorTitle = "Title",
	GatorSection = "Statement",
	GatorLabel = "Identifier",
	GatorAction = "Function",
	GatorKeymap = "Special",
	GatorSuccess = "DiagnosticOk",
	GatorWarning = "DiagnosticWarn",
	GatorError = "DiagnosticError",
	GatorMuted = "Comment",
}

local words = {
	GatorSuccess = { "ok", "ready", "available", "completed", "passed", "accepted", "enabled" },
	GatorWarning = { "loading", "recovering", "degraded", "pending", "unverified", "unavailable", "outside" },
	GatorError = { "failed", "error", "denied", "rejected", "cancelled", "canceled" },
	GatorMuted = { "empty", "none", "optional", "disabled" },
}

local function setup()
	if configured then
		return
	end
	for name, link in pairs(groups) do
		vim.api.nvim_set_hl(0, name, { default = true, link = link })
	end
	configured = true
end

local function add(buffer, group, row, first, last)
	if first < last then
		vim.api.nvim_buf_add_highlight(buffer, namespace, group, row, first, last)
	end
end

local function matches(buffer, row, line, word, group)
	local start = 1
	local source = line:lower()
	local pattern = "%f[%a]" .. word .. "%f[^%a]"
	while true do
		local first, last = source:find(pattern, start)
		if not first then
			return
		end
		add(buffer, group, row, first - 1, last)
		start = last + 1
	end
end

local function keymaps(buffer, row, line)
	local start = 1
	while true do
		local first, last = line:find("<[^>]+>", start)
		if not first then
			break
		end
		add(buffer, "GatorKeymap", row, first - 1, last)
		start = last + 1
	end
	for _, keymap in ipairs({ "j/k", "q", "?", "a/r", "<Space>" }) do
		matches(buffer, row, line, keymap:lower(), "GatorKeymap")
	end
end

function M.apply(buffer, lines)
	if type(buffer) ~= "number" or not vim.api.nvim_buf_is_valid(buffer) then
		error("Gator highlights require a valid buffer", 2)
	end
	if type(lines) ~= "table" or not vim.islist(lines) then
		error("Gator highlights require a line array", 2)
	end
	setup()
	vim.api.nvim_buf_clear_namespace(buffer, namespace, 0, -1)
	for index, line in ipairs(lines) do
		local row = index - 1
		if line:match("^Gator ") or line:match("^%S+ Gator ") then
			add(buffer, "GatorTitle", row, 0, #line)
		elseif line:match("^%s*[%a][%w _/-]*:%s*$") then
			add(buffer, "GatorSection", row, 0, #line)
		else
			local first, last = line:find("^%s*[%a][%w _/-]*:")
			if first then
				add(buffer, "GatorLabel", row, first - 1, last)
			end
		end
		if line:match("^%s*>") then
			add(buffer, "GatorAction", row, 0, #line)
		end
		for group, values in pairs(words) do
			for _, word in ipairs(values) do
				matches(buffer, row, line, word, group)
			end
		end
		keymaps(buffer, row, line)
	end
end

vim.api.nvim_create_autocmd("ColorScheme", {
	group = vim.api.nvim_create_augroup("GatorTextHighlights", { clear = true }),
	callback = function()
		configured = false
		setup()
	end,
})

return M
