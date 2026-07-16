local run = require("gator.core.run")
local M = {}
local reviews = {}

local function fail(message)
	error("Gator diff review: " .. message, 3)
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function changes(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("changes must be an array")
	end
	local result = {}
	for index, change in ipairs(value) do
		if type(change) ~= "table" then
			fail("change " .. index .. " must be a table")
		end
		for key in pairs(change) do
			if key ~= "path" and key ~= "before" and key ~= "after" then
				fail("change " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		if type(change.before) ~= "string" or type(change.after) ~= "string" then
			fail("change " .. index .. " before and after content must be strings")
		end
		result[index] = {
			path = require_string(change.path, "change " .. index .. " path"),
			before = change.before,
			after = change.after,
		}
	end
	return result
end

local function current()
	local tabpage = vim.api.nvim_get_current_tabpage()
	local review = reviews[tabpage]
	if review and vim.api.nvim_win_is_valid(review.window) then
		return review, tabpage
	end
	reviews[tabpage] = nil
	return nil, tabpage
end

local function close_diffs(review)
	for _, diff in ipairs(review.diffs) do
		for _, window in ipairs({ diff.before, diff.after }) do
			if vim.api.nvim_win_is_valid(window) then
				vim.api.nvim_win_close(window, true)
			end
		end
	end
	review.diffs = {}
end

local function render(review)
	local lines = { "Gator review · task " .. review.run.task_id .. " · run " .. review.run.id }
	if #review.changes == 0 then
		table.insert(lines, "No changed files")
	else
		for index, change in ipairs(review.changes) do
			local marker = index == review.selected and ">" or " "
			table.insert(lines, marker .. " " .. change.path)
		end
	end
	vim.api.nvim_buf_set_lines(review.buffer, 0, -1, false, lines)
end

local function scratch(review, side, content)
	review.sequence = review.sequence + 1
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.api.nvim_buf_set_name(buffer, "gator://review/" .. review.sequence .. "/" .. side)
	vim.bo[buffer].buftype = "nofile"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_buf_set_lines(buffer, 0, -1, false, vim.split(content, "\n", { plain = true, trimempty = false }))
	vim.bo[buffer].modifiable = false
	return buffer
end

function M.open(opts)
	if type(opts) ~= "table" or not run.is(opts.run) then
		fail("open requires a Gator run")
	end
	local value = changes(opts.changes or {})
	local review, tabpage = current()
	if review then
		close_diffs(review)
		review.run = opts.run
		review.changes = value
		review.selected = math.min(review.selected, math.max(#value, 1))
		render(review)
		vim.api.nvim_set_current_win(review.window)
		return review.window
	end
	vim.cmd("botright 12new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].filetype = "gator-review"
	vim.bo[buffer].bufhidden = "wipe"
	vim.api.nvim_win_set_buf(window, buffer)
	review =
		{ window = window, buffer = buffer, run = opts.run, changes = value, selected = 1, diffs = {}, sequence = 0 }
	reviews[tabpage] = review
	render(review)
	return window
end

function M.select(index)
	local review = current()
	if not review then
		fail("no diff review is open in this tab")
	end
	if type(index) ~= "number" or index % 1 ~= 0 or index < 1 or index > #review.changes then
		fail("selection must identify a changed file")
	end
	review.selected = index
	render(review)
	return vim.deepcopy(review.changes[index])
end

function M.open_selected()
	local review = current()
	if not review then
		fail("no diff review is open in this tab")
	end
	local change = review.changes[review.selected]
	if not change then
		fail("no changed file is selected")
	end
	vim.cmd("botright 10new")
	local after = vim.api.nvim_get_current_win()
	vim.cmd("leftabove vsplit")
	local before = vim.api.nvim_get_current_win()
	vim.api.nvim_win_set_buf(before, scratch(review, "before", change.before))
	vim.api.nvim_win_set_buf(after, scratch(review, "after", change.after))
	vim.wo[before].diff = true
	vim.wo[after].diff = true
	local result = { before = before, after = after, path = change.path }
	table.insert(review.diffs, result)
	return vim.deepcopy(result)
end

function M.close()
	local review, tabpage = current()
	if not review then
		return false
	end
	close_diffs(review)
	if vim.api.nvim_win_is_valid(review.window) then
		vim.api.nvim_win_close(review.window, true)
	end
	reviews[tabpage] = nil
	return true
end

return M
