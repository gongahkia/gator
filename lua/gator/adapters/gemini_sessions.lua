local M = {}

local function fail(message)
	error("Gator Gemini sessions: " .. message, 3)
end

local function id(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty opaque session id")
	end
	return value
end

local function run(callback, argv)
	local ok, value = pcall(callback, argv)
	if not ok or type(value) ~= "table" or value.code ~= 0 or type(value.stdout) ~= "string" then
		fail("Gemini session request failed")
	end
	return value
end

local function result(callback, argv)
	local output = run(callback, argv).stdout
	for line in vim.gsplit(output, "\n", { plain = true, trimempty = true }) do
		local ok, event = pcall(vim.json.decode, line)
		if ok and type(event) == "table" and event.type == "init" and type(event.session_id) == "string" then
			return { provider = "gemini", id = id(event.session_id, "session_id"), owner = "provider" }
		end
	end
	fail("Gemini stream result does not contain an init session id")
end

function M.create(opts)
	if type(opts) ~= "table" or type(opts.run) ~= "function" or type(opts.prompt) ~= "string" or opts.prompt == "" then
		fail("create requires run and prompt")
	end
	return result(opts.run, { opts.executable or "gemini", "--prompt", opts.prompt, "--output-format", "stream-json" })
end

function M.list(opts)
	if type(opts) ~= "table" or type(opts.run) ~= "function" then
		fail("list requires run")
	end
	local output = run(opts.run, { opts.executable or "gemini", "--list-sessions" }).stdout
	if output:match("^No previous sessions found for this project%.%s*$") then
		return {}
	end
	local sessions = {}
	for index, line in ipairs(vim.split(output, "\n", { plain = true, trimempty = true })) do
		local number, session_id = line:match("^%s*(%d+)%..*%[([%w_-]+)%]%s*$")
		if number and session_id then
			sessions[#sessions + 1] = {
				provider = "gemini",
				id = id(session_id, "session " .. number .. " id"),
				owner = "provider",
			}
		end
	end
	if #sessions == 0 then
		fail("Gemini session list returned an unsupported format")
	end
	return sessions
end

function M.resume(opts)
	if type(opts) ~= "table" or type(opts.run) ~= "function" or type(opts.prompt) ~= "string" or opts.prompt == "" then
		fail("resume requires run and prompt")
	end
	return result(opts.run, {
		opts.executable or "gemini",
		"--resume",
		id(opts.id, "id"),
		"--prompt",
		opts.prompt,
		"--output-format",
		"stream-json",
	})
end

function M.close(opts)
	if type(opts) ~= "table" or type(opts.run) ~= "function" then
		fail("close requires run")
	end
	local output = run(opts.run, { opts.executable or "gemini", "--delete-session", id(opts.id, "id") }).stdout
	if not output:match("^Deleted session ") then
		fail("Gemini session deletion was not confirmed")
	end
	return true
end

return M
