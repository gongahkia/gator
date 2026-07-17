local stream = require("gator.adapters.amp_stream")
local M = {}

local function fail(message)
	error("Gator Amp sessions: " .. message, 3)
end

local function id(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty opaque thread id")
	end
	return value
end

local function options(value, name)
	if
		type(value) ~= "table"
		or type(value.run) ~= "function"
		or type(value.prompt) ~= "string"
		or value.prompt == ""
	then
		fail(name .. " requires run and prompt")
	end
	return value
end

local function execute(run, argv)
	local ok, value = pcall(run, argv)
	if not ok or type(value) ~= "table" or value.code ~= 0 or type(value.stdout) ~= "string" then
		fail("Amp thread request failed")
	end
	local result = stream.parse(value.stdout)
	return { provider = "amp", id = result.id, owner = "provider" }
end

function M.create(opts)
	opts = options(opts, "create")
	return execute(opts.run, {
		opts.executable or "amp",
		"--execute",
		opts.prompt,
		"--stream-json",
		"--no-archive-after-execute",
	})
end

function M.list()
	return { available = false, reason = "Amp CLI does not document a machine-readable thread-list schema" }
end

function M.resume(opts)
	opts = options(opts, "resume")
	return execute(opts.run, {
		opts.executable or "amp",
		"threads",
		"continue",
		id(opts.id, "id"),
		"--execute",
		opts.prompt,
		"--stream-json",
		"--no-archive-after-execute",
	})
end

function M.close(opts)
	if type(opts) ~= "table" or type(opts.run) ~= "function" then
		fail("close requires run and id")
	end
	local ok, value = pcall(opts.run, { opts.executable or "amp", "threads", "delete", id(opts.id, "id") })
	if not ok or type(value) ~= "table" or value.code ~= 0 then
		fail("Amp thread deletion failed")
	end
	return true
end

return M
