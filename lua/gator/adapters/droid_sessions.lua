local M = {}

local function fail(message)
	error("Gator Droid sessions: " .. message, 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

function M.command(opts)
	if type(opts) ~= "table" then
		fail("command requires cwd")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "executable" then
			fail("command contains unsupported field: " .. tostring(key))
		end
	end
	return {
		opts.executable or "droid",
		"exec",
		"--cwd",
		text(opts.cwd, "cwd"),
		"--input-format",
		"stream-jsonrpc",
		"--output-format",
		"stream-jsonrpc",
	}
end

function M.create(opts)
	if type(opts) ~= "table" then
		fail("create requires cwd")
	end
	for key in pairs(opts) do
		if key ~= "cwd" and key ~= "machine_id" then
			fail("create contains unsupported field: " .. tostring(key))
		end
	end
	return {
		method = "droid.initialize_session",
		params = { machineId = opts.machine_id or "gator", cwd = text(opts.cwd, "cwd"), autonomyLevel = "off" },
	}
end

function M.resume(opts)
	if type(opts) ~= "table" then
		fail("resume requires id")
	end
	for key in pairs(opts) do
		if key ~= "id" then
			fail("resume contains unsupported field: " .. tostring(key))
		end
	end
	return { method = "droid.load_session", params = { sessionId = text(opts.id, "id") } }
end

function M.prompt(opts)
	if type(opts) ~= "table" then
		fail("prompt requires text")
	end
	for key in pairs(opts) do
		if key ~= "text" then
			fail("prompt contains unsupported field: " .. tostring(key))
		end
	end
	return { method = "droid.add_user_message", params = { text = text(opts.text, "text") } }
end

function M.interrupt()
	return { method = "droid.interrupt_session", params = vim.empty_dict() }
end

function M.close_session(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("close_session requires optional reason")
	end
	for key in pairs(opts) do
		if key ~= "reason" then
			fail("close_session contains unsupported field: " .. tostring(key))
		end
	end
	local reason = opts.reason or "other"
	if reason ~= "clear" and reason ~= "logout" and reason ~= "prompt_input_exit" and reason ~= "other" then
		fail("close_session reason is unsupported")
	end
	return { method = "droid.close_session", params = { reason = reason } }
end

function M.list()
	return { available = false, reason = "Droid session discovery is not part of the managed JSON-RPC bridge" }
end

function M.close()
	return { available = false, reason = "Droid session deletion is not exposed by Gator" }
end

return M
