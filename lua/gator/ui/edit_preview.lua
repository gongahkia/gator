local accessibility = require("gator.ui.accessibility")
local panel_window = require("gator.ui.window")

local M = {}
local panels = {}

function M.open(opts)
	if
		type(opts) ~= "table"
		or type(opts.before) ~= "string"
		or type(opts.after) ~= "string"
		or type(opts.on_apply) ~= "function"
	then
		error("Gator edit preview: open requires before, after, and apply callback", 3)
	end
	local tabpage = vim.api.nvim_get_current_tabpage()
	if panels[tabpage] then
		return false
	end
	local opened = panel_window.open("botright 18new")
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(opened.window, buffer)
	local diff = vim.diff(opts.before, opts.after, { result_type = "unified" })
	local panel = {
		window = opened.window,
		buffer = buffer,
		previous = opened.previous,
		on_apply = opts.on_apply,
		on_cancel = opts.on_cancel or function() end,
	}
	panels[tabpage] = panel
	local lines = { "Gator selection edit preview", "" }
	vim.list_extend(lines, diff == "" and { "No textual change" } or vim.split(diff, "\n", { plain = true }))
	vim.list_extend(lines, { "", "<CR> apply · q cancel · ? help" })
	accessibility.render(buffer, lines, "gator-edit-preview")
	local function close(apply)
		if panels[tabpage] ~= panel then
			return false
		end
		panels[tabpage] = nil
		panel_window.close(panel.window, panel.previous)
		if apply then
			panel.on_apply()
		else
			panel.on_cancel()
		end
		return true
	end
	accessibility.panel(buffer, { accept = "<CR>", reject = "q", help = "?" }, {
		accept = function()
			close(true)
		end,
		reject = function()
			close(false)
		end,
		help = function()
			require("gator.ui.notice").show(
				"Gator edit: <CR> applies the shown replacement, q cancels",
				vim.log.levels.INFO
			)
		end,
	})
	return panel.window
end

return M
