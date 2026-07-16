local M = {}

local function fail(message)
	error("Gator Claude sessions: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty opaque session id")
	end
	return value
end

local function result(run, argv)
	local ok, value = pcall(run, argv)
	if not ok or type(value) ~= "table" or value.code ~= 0 or type(value.stdout) ~= "string" then
		fail("Claude print-mode session request failed")
	end
	local decoded_ok, document = pcall(vim.json.decode, value.stdout)
	if not decoded_ok or type(document) ~= "table" or document.is_error or type(document.session_id) ~= "string" then
		fail("Claude print-mode result does not contain a successful session id")
	end
	return { provider = "claude", id = document.session_id, owner = "provider" }
end

local function options(opts, name)
	if type(opts) ~= "table" or type(opts.run) ~= "function" or type(opts.prompt) ~= "string" or opts.prompt == "" then
		fail(name .. " requires run and prompt")
	end
	return opts
end

function M.create(opts)
	opts = options(opts, "create")
	return result(opts.run, { opts.executable or "claude", "-p", opts.prompt, "--output-format", "json" })
end

function M.resume(opts)
	opts = options(opts, "resume")
	return result(opts.run, {
		opts.executable or "claude",
		"-p",
		"--resume",
		identifier(opts.id, "id"),
		opts.prompt,
		"--output-format",
		"json",
	})
end

function M.list()
	return { available = false, reason = "Claude Code CLI does not expose a session-list API" }
end

function M.close()
	return { available = false, reason = "Claude Code CLI does not expose a session-close API" }
end

return M
