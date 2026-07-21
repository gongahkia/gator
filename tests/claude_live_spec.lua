if vim.env.GATOR_LIVE_CLAUDE ~= "1" then
	return
end

local result = vim.system({ "claude", "auth", "status" }, { text = true }):wait()
assert(result.code == 0, "protected Claude verification requires an existing CLI-owned login")
local ok, status = pcall(vim.json.decode, result.stdout or "")
assert(
	ok and type(status) == "table" and status.loggedIn == true,
	"protected Claude verification must prove the CLI-owned login state"
)
local help = vim.system({ "claude", "--help" }, { text = true }):wait()
assert(
	help.code == 0
		and (help.stdout or ""):find("--input-format", 1, true)
		and (help.stdout or ""):find("--output-format", 1, true)
		and (help.stdout or ""):find("stream-json", 1, true)
		and (help.stdout or ""):find("--verbose", 1, true),
	"protected Claude verification requires structured stream support"
)

if vim.env.GATOR_LIVE_CLAUDE_AUTH ~= "1" then
	return
end

local redact = require("gator.policy.redact")
local uv = vim.uv
local stdin, stdout, stderr = uv.new_pipe(false), uv.new_pipe(false), uv.new_pipe(false)
local buffer = ""
local session_id
local native_session_id
local exited = false
local handle
local function close()
	if handle and not handle:is_closing() then
		handle:kill("sigterm")
	end
	for _, pipe in ipairs({ stdin, stdout, stderr }) do
		if pipe and not pipe:is_closing() then
			pipe:close()
		end
	end
end
local function record(data)
	buffer = buffer .. data
	while true do
		local ending = buffer:find("\n", 1, true)
		if not ending then
			return
		end
		local line = buffer:sub(1, ending - 1)
		buffer = buffer:sub(ending + 1)
		local ok, value = pcall(vim.json.decode, line)
		if ok and type(value) == "table" then
			if value.type == "system" and value.subtype == "init" then
				if type(value.session_id) == "string" and value.session_id ~= "" then
					native_session_id = value.session_id
				end
			end
		end
	end
end
local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Claude verification requires a temporary workspace")
local ok, failure = xpcall(function()
	local created = vim.system({
		"claude",
		"-p",
		"--output-format",
		"json",
		"--permission-mode",
		"plan",
		"--max-turns",
		"1",
		"Reply only with READY. Do not use tools or edit files.",
	}, { cwd = workspace, text = true, timeout = 30000 }):wait()
	if created.code ~= 0 then
		error(
			"authenticated Claude verification requires a native session (exit "
				.. created.code
				.. "): "
				.. redact.text((created.stderr or "") .. "\n" .. (created.stdout or ""))
		)
	end
	local decoded, result = pcall(vim.json.decode, created.stdout or "")
	assert(
		decoded and type(result) == "table" and type(result.session_id) == "string" and result.session_id ~= "",
		"authenticated Claude verification requires a persisted provider-owned session"
	)
	session_id = result.session_id
	handle = assert(uv.spawn("claude", {
		args = {
			"-p",
			"--resume",
			session_id,
			"--input-format",
			"stream-json",
			"--output-format",
			"stream-json",
			"--verbose",
			"--permission-mode",
			"plan",
			"--max-turns",
			"1",
		},
		cwd = workspace,
		stdio = { stdin, stdout, stderr },
	}, function()
		exited = true
	end))
	stdout:read_start(function(error, data)
		assert(not error, "authenticated Claude verification received stream stdout failure")
		if data then
			record(data)
		end
	end)
	stderr:read_start(function() end)
	stdin:write(vim.json.encode({
		type = "user",
		message = {
			role = "user",
			content = {
				{
					type = "text",
					text = "Do not use tools or edit files. Write a detailed analysis of deterministic testing.",
				},
			},
		},
	}) .. "\n")
	assert(
		vim.wait(20000, function()
			return native_session_id ~= nil
		end),
		"authenticated Claude verification requires native resumed session initialization"
	)
	assert(native_session_id == session_id, "authenticated Claude verification must retain provider session ownership")
	vim.wait(1000, function()
		return false
	end)
	assert(not exited, "authenticated Claude verification requires an active native query before interruption")
	assert(handle:kill("sigint"), "authenticated Claude verification requires native query interruption")
	assert(
		vim.wait(10000, function()
			return exited
		end),
		"authenticated Claude verification requires interrupted query exit"
	)
	local resumed = vim.system({
		"claude",
		"-p",
		"--resume",
		session_id,
		"--output-format",
		"json",
		"--permission-mode",
		"plan",
		"--max-turns",
		"1",
		"Reply only with RECOVERED. Do not use tools or edit files.",
	}, { cwd = workspace, text = true, timeout = 30000 }):wait()
	if resumed.code ~= 0 then
		error(
			"authenticated Claude verification requires native resume (exit "
				.. resumed.code
				.. "): "
				.. redact.text(resumed.stderr or "")
		)
	end
	local decoded, result = pcall(vim.json.decode, resumed.stdout or "")
	assert(
		decoded and type(result) == "table" and result.session_id == session_id,
		"authenticated Claude verification must preserve native session ownership through interruption and recovery"
	)
end, debug.traceback)
close()
vim.fn.delete(workspace, "d")
assert(ok, failure)
