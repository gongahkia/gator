local M = {}
local Lifecycle = {}
Lifecycle.__index = Lifecycle

local function fail(message)
	error("Gator indexer lifecycle: " .. message, 3)
end

function M.new(opts)
	if
		type(opts) ~= "table"
		or type(opts.manager) ~= "table"
		or type(opts.manager.launch) ~= "function"
		or type(opts.manager.status) ~= "function"
		or type(opts.manager.cancel) ~= "function"
		or type(opts.manager.restart) ~= "function"
	then
		fail("new requires a process manager")
	end
	if type(opts.cwd) ~= "string" or opts.cwd == "" or type(opts.executable) ~= "string" or opts.executable == "" then
		fail("new requires cwd and executable")
	end
	local cwd = vim.uv.fs_realpath(opts.cwd)
	if not cwd or vim.fn.isdirectory(cwd) ~= 1 then
		fail("cwd must be an existing directory")
	end
	return setmetatable(
		{ manager = opts.manager, cwd = cwd, executable = opts.executable, id = opts.id or "gator-index" },
		Lifecycle
	)
end

function Lifecycle:start()
	local status = self.manager:status(self.id)
	if not status then
		return self.manager:launch({ id = self.id, command = { self.executable }, cwd = self.cwd })
	end
	if status.state == "running" then
		return status
	end
	if status.state == "failed" or status.state == "completed" or status.state == "cancelled" then
		return self.manager:restart(self.id)
	end
	fail("indexer is transitioning")
end

function Lifecycle:stop()
	local status = self.manager:status(self.id)
	if not status then
		return false
	end
	if status.state ~= "running" then
		return false
	end
	return self.manager:cancel(self.id)
end

function Lifecycle:recover()
	return self:start()
end

function Lifecycle:status()
	return self.manager:status(self.id)
end

return M
