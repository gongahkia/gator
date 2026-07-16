local M = {}
local Manager = {}

Manager.__index = Manager

local function fail(message)
	error("Gator terminal: " .. message, 3)
end

local function identifier(value)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail("id must be a lowercase identifier")
	end
	return value
end

local function command(value)
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 then
		fail("command must be a non-empty argv array")
	end
	for index, item in ipairs(value) do
		if type(item) ~= "string" or item == "" then
			fail("command argument " .. index .. " must be a non-empty string")
		end
	end
	return value
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("options must be a table")
	end
	local termopen = opts.termopen
	if termopen == false then
		termopen = nil
	elseif termopen == nil and vim.fn.exists("*termopen") == 1 then
		termopen = vim.fn.termopen
	end
	if termopen ~= nil and type(termopen) ~= "function" then
		fail("termopen must be a function")
	end
	if opts.jobstop ~= nil and type(opts.jobstop) ~= "function" then
		fail("jobstop must be a function")
	end
	return setmetatable({ termopen = termopen, jobstop = opts.jobstop or vim.fn.jobstop, sessions = {} }, Manager)
end

function Manager:inspect()
	if self.termopen then
		return { available = true, mode = "nvim_terminal" }
	end
	return { available = false, reason = "Neovim terminal support is unavailable" }
end

function Manager:open(opts)
	if not self.termopen then
		fail("terminal fallback is unavailable: " .. self:inspect().reason)
	end
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	local id = identifier(opts.id)
	if self.sessions[id] then
		fail("terminal session is already open: " .. id)
	end
	if opts.cwd ~= nil and (type(opts.cwd) ~= "string" or opts.cwd == "") then
		fail("cwd must be a non-empty string")
	end
	vim.cmd("botright 16new")
	local window = vim.api.nvim_get_current_win()
	local buffer = vim.api.nvim_get_current_buf()
	local job_id = self.termopen(command(opts.command), { cwd = opts.cwd })
	if type(job_id) ~= "number" or job_id < 1 then
		vim.api.nvim_win_close(window, true)
		fail("terminal launch failed")
	end
	self.sessions[id] = { window = window, buffer = buffer, job_id = job_id }
	return { id = id, window = window, buffer = buffer, job_id = job_id }
end

function Manager:attach(id)
	id = identifier(id)
	local session = self.sessions[id]
	if not session or not vim.api.nvim_win_is_valid(session.window) then
		fail("terminal session is not attachable: " .. id)
	end
	vim.api.nvim_set_current_win(session.window)
	return session.window
end

function Manager:close(id)
	id = identifier(id)
	local session = self.sessions[id]
	if not session then
		return false
	end
	self.jobstop(session.job_id)
	if vim.api.nvim_win_is_valid(session.window) then
		vim.api.nvim_win_close(session.window, true)
	end
	self.sessions[id] = nil
	return true
end

return M
