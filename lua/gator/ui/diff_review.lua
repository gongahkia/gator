local run = require("gator.core.run")
local M = {}
local reviews = {}
local decisions = { pending = true, accepted = true, rejected = true }

local function fail(message)
	error("Gator diff review: " .. message, 3)
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function hunks(change)
	if type(vim.diff) ~= "function" then
		fail("native diff capability is unavailable")
	end
	local output = vim.diff(change.before, change.after, { result_type = "unified", ctxlen = 0 })
	local result = {}
	for header in output:gmatch("[^\n]+") do
		local before_start, before_count, after_start, after_count =
			header:match("^@@ %-(%d+),?(%d*) %+(%d+),?(%d*) @@")
		if before_start then
			before_count = before_count == "" and 1 or tonumber(before_count)
			after_count = after_count == "" and 1 or tonumber(after_count)
			table.insert(result, {
				id = "hunk-" .. vim.fn.sha256(change.path .. "\0" .. header):sub(1, 16),
				before_start = tonumber(before_start),
				before_count = before_count,
				after_start = tonumber(after_start),
				after_count = after_count,
				decision = "pending",
			})
		end
	end
	return result
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
		result[index].hunks = hunks(result[index])
	end
	return result
end

local function selected_hunk(review)
	local change = review.changes[review.selected]
	return change and change.hunks[review.hunk] or nil
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
			for hunk_index, hunk in ipairs(change.hunks) do
				local hunk_marker = index == review.selected and hunk_index == review.hunk and ">" or " "
				local annotation = hunk.annotation and " · " .. hunk.annotation or ""
				table.insert(
					lines,
					"  "
						.. hunk_marker
						.. " @@ -"
						.. hunk.before_start
						.. ","
						.. hunk.before_count
						.. " +"
						.. hunk.after_start
						.. ","
						.. hunk.after_count
						.. " · "
						.. hunk.decision
						.. annotation
				)
			end
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
		review.hunk = 1
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
	review = {
		window = window,
		buffer = buffer,
		run = opts.run,
		changes = value,
		selected = 1,
		hunk = 1,
		diffs = {},
		sequence = 0,
	}
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
	review.hunk = 1
	render(review)
	return vim.deepcopy(review.changes[index])
end

function M.hunk()
	local review = current()
	if not review then
		fail("no diff review is open in this tab")
	end
	local hunk = selected_hunk(review)
	if not hunk then
		fail("no hunk is selected")
	end
	return vim.deepcopy(hunk)
end

function M.next_hunk()
	local review = current()
	if not review then
		fail("no diff review is open in this tab")
	end
	for change_index = review.selected, #review.changes do
		local start = change_index == review.selected and review.hunk + 1 or 1
		if review.changes[change_index].hunks[start] then
			review.selected, review.hunk = change_index, start
			render(review)
			return M.hunk()
		end
	end
	fail("no next hunk is available")
end

function M.previous_hunk()
	local review = current()
	if not review then
		fail("no diff review is open in this tab")
	end
	for change_index = review.selected, 1, -1 do
		local start = change_index == review.selected and review.hunk - 1 or #review.changes[change_index].hunks
		if review.changes[change_index].hunks[start] then
			review.selected, review.hunk = change_index, start
			render(review)
			return M.hunk()
		end
	end
	fail("no previous hunk is available")
end

function M.stage(decision)
	if type(decision) ~= "string" or not decisions[decision] then
		fail("decision must be pending, accepted, or rejected")
	end
	local review = current()
	if not review then
		fail("no diff review is open in this tab")
	end
	local hunk = selected_hunk(review)
	if not hunk then
		fail("no hunk is selected")
	end
	hunk.decision = decision
	render(review)
	return vim.deepcopy(hunk)
end

function M.annotate(annotation)
	annotation = require_string(annotation, "annotation")
	local review = current()
	if not review then
		fail("no diff review is open in this tab")
	end
	local hunk = selected_hunk(review)
	if not hunk then
		fail("no hunk is selected")
	end
	hunk.annotation = annotation
	render(review)
	return vim.deepcopy(hunk)
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
	local hunk = selected_hunk(review)
	if hunk then
		local before_line =
			math.min(math.max(hunk.before_start, 1), vim.api.nvim_buf_line_count(vim.api.nvim_win_get_buf(before)))
		local after_line =
			math.min(math.max(hunk.after_start, 1), vim.api.nvim_buf_line_count(vim.api.nvim_win_get_buf(after)))
		vim.api.nvim_win_set_cursor(before, { before_line, 0 })
		vim.api.nvim_win_set_cursor(after, { after_line, 0 })
	end
	local result = { before = before, after = after, path = change.path, hunk = hunk and hunk.id or nil }
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
