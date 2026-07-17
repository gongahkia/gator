local stream = require("gator.adapters.cursor_stream")
local M = {}

local function fail(message)
	error("Gator Cursor sessions: " .. message, 3)
end

local function id(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty opaque session id")
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
		fail("Cursor session request failed")
	end
	local result = stream.parse(value.stdout)
	return { provider = "cursor", id = result.id, owner = "provider" }
end

function M.create(opts)
	opts = options(opts, "create")
	return execute(
		opts.run,
		{ opts.executable or "cursor-agent", "--print", "--output-format", "stream-json", opts.prompt }
	)
end

function M.list()
	return { available = false, reason = "Cursor ls does not document a machine-readable session-list schema" }
end

function M.resume(opts)
	opts = options(opts, "resume")
	return execute(opts.run, {
		opts.executable or "cursor-agent",
		"--print",
		"--output-format",
		"stream-json",
		"--resume",
		id(opts.id, "id"),
		opts.prompt,
	})
end

function M.close()
	return { available = false, reason = "Cursor CLI does not document provider-owned session deletion or close" }
end

return M
