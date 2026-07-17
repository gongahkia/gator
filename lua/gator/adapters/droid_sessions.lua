local stream = require("gator.adapters.droid_stream")
local M = {}

local function fail(message)
	error("Gator Droid sessions: " .. message, 3)
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
		fail("Droid session request failed")
	end
	local result = stream.parse(value.stdout)
	if result.is_error then
		fail("Droid session returned an error result")
	end
	return { provider = "droid", id = result.id, owner = "provider" }
end

function M.create(opts)
	opts = options(opts, "create")
	if type(opts.cwd) ~= "string" or opts.cwd == "" then
		fail("create requires cwd")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "prompt" and key ~= "cwd" and key ~= "executable" then
			fail("create contains unsupported field: " .. tostring(key))
		end
	end
	return execute(
		opts.run,
		{ opts.executable or "droid", "exec", "--cwd", opts.cwd, "--output-format", "json", opts.prompt }
	)
end

function M.list()
	return { available = false, reason = "Droid CLI does not document a machine-readable session-list command schema" }
end

function M.resume(opts)
	opts = options(opts, "resume")
	for key in pairs(opts) do
		if key ~= "run" and key ~= "id" and key ~= "prompt" and key ~= "executable" then
			fail("resume contains unsupported field: " .. tostring(key))
		end
	end
	return execute(opts.run, {
		opts.executable or "droid",
		"exec",
		"--session-id",
		id(opts.id, "id"),
		"--output-format",
		"json",
		opts.prompt,
	})
end

function M.fork(opts)
	opts = options(opts, "fork")
	for key in pairs(opts) do
		if key ~= "run" and key ~= "id" and key ~= "prompt" and key ~= "executable" then
			fail("fork contains unsupported field: " .. tostring(key))
		end
	end
	return execute(opts.run, {
		opts.executable or "droid",
		"exec",
		"--fork",
		id(opts.id, "id"),
		"--output-format",
		"json",
		opts.prompt,
	})
end

function M.close()
	return { available = false, reason = "Droid CLI does not document provider-owned session close or deletion" }
end

return M
