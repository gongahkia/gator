local redact = require("gator.policy.redact")
local droid_stream = require("gator.adapters.droid_stream")

local M = { api_version = 1 }
local Manager = {}

Manager.__index = Manager

M.profiles = {
	aider = { mode = "history", executable = "aider", adapter = "gator.adapters.aider" },
	amp = { mode = "stream", executable = "amp", adapter = "gator.adapters.amp" },
	cline = { mode = "acp", executable = "cline", adapter = "gator.adapters.cline", command = { "cline", "--acp" } },
	copilot = {
		mode = "acp",
		executable = "copilot",
		adapter = "gator.adapters.copilot",
		command = { "copilot", "--acp", "--stdio" },
		resume = false,
	},
	cursor = { mode = "stream", executable = "cursor-agent", adapter = "gator.adapters.cursor" },
	droid = { mode = "json", executable = "droid", adapter = "gator.adapters.droid" },
	gemini = { mode = "acp", executable = "gemini", adapter = "gator.adapters.gemini", command = { "gemini", "--acp" } },
	goose = { mode = "acp", executable = "goose", adapter = "gator.adapters.goose", command = { "goose", "acp" } },
	kimi = { mode = "acp", executable = "kimi", adapter = "gator.adapters.kimi", command = { "kimi", "acp" } },
	vibe = { mode = "acp", executable = "vibe", adapter = "gator.adapters.vibe", command = { "vibe-acp" } },
}

local function probe_options(value, cwd)
	local result = {
		run = function(argv, input)
			local process = vim.system(argv, { cwd = cwd, text = true, stdin = input, timeout = 3000 }):wait()
			return { code = process.code, stdout = process.stdout or "" }
		end,
	}
	if value.mode == "acp" and value.executable == "cline" then
		result.cwd = cwd
	end
	return result
end

local function profile_supported(value, probe)
	local capabilities = type(probe.capabilities) == "table" and probe.capabilities or {}
	if value.mode == "acp" then
		return capabilities.acp == true
	end
	if value.mode == "stream" and value.executable == "amp" then
		return capabilities.stream_json == true and capabilities.stream_input == true
	end
	if value.mode == "stream" and value.executable == "cursor-agent" then
		return capabilities.structured_output == true and capabilities.session_resume == true
	end
	if value.mode == "json" then
		return capabilities.execute == true and capabilities.session_resume == true
	end
	return capabilities.message == true and capabilities.history == true
end

local function fail(message)
	error("Gator managed sessions: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function directory(value)
	value = text(value, "cwd")
	value = vim.uv.fs_realpath(value)
	if not value or vim.fn.isdirectory(value) ~= 1 then
		fail("cwd must resolve to a directory")
	end
	return value
end

local function profile(name)
	name = text(name, "provider")
	local value = M.profiles[name]
	if not value then
		fail("provider is unavailable for managed sessions: " .. name)
	end
	return name, value
end

local function session(value, provider_name, mode)
	if value == nil then
		return nil
	end
	if type(value) ~= "table" or value.provider ~= provider_name or type(value.id) ~= "string" or value.id == "" then
		fail("session must identify the managed provider")
	end
	if value.owner ~= "provider" and value.owner ~= "gator" then
		fail("session owner is unsupported")
	end
	if value.mode ~= nil and value.mode ~= mode then
		fail("session mode does not match provider")
	end
	if value.owner == "gator" and mode ~= "history" then
		fail("gator-owned sessions must use history mode")
	end
	return { provider = provider_name, id = value.id, owner = value.owner, mode = mode }
end

local function callback(value, name)
	if value ~= nil and type(value) ~= "function" then
		fail(name .. " must be a function")
	end
	return value or function() end
end

local function command(value)
	local result = {}
	for index, item in ipairs(value) do
		result[index] = text(item, "command argument " .. index)
	end
	return result
end

local function default_spawn(argv, opts, done)
	return vim.system(argv, {
		cwd = opts.cwd,
		text = true,
		stdin = true,
		stdout = opts.stdout,
		stderr = opts.stderr,
	}, done)
end

local function profile_command(provider_name, value, existing, prompt, history, cwd)
	if value.mode == "acp" then
		return command(value.command)
	end
	if value.mode == "stream" and provider_name == "amp" then
		local argv = existing and { "amp", "threads", "continue", existing.id } or { "amp" }
		vim.list_extend(argv, { "--execute", "--stream-json", "--stream-json-input", "--no-archive-after-execute" })
		return argv
	end
	if value.mode == "stream" and provider_name == "cursor" then
		local argv = { "cursor-agent", "--print", "--output-format", "stream-json" }
		if existing then
			vim.list_extend(argv, { "--resume", existing.id })
		end
		table.insert(argv, prompt)
		return argv
	end
	if value.mode == "json" then
		local argv = { "droid", "exec", "--cwd", cwd, "--output-format", "json" }
		if existing then
			vim.list_extend(argv, { "--session-id", existing.id })
		end
		table.insert(argv, prompt)
		return argv
	end
	if value.mode == "history" then
		local argv = { "aider", "--chat-history-file", history, "--message", prompt }
		if existing then
			table.insert(argv, "--restore-chat-history")
		end
		return argv
	end
	fail("provider command is unavailable")
end

local function acp_text(params)
	local update = type(params) == "table" and params.update
	if type(update) ~= "table" then
		return nil
	end
	if
		update.sessionUpdate == "agent_message_chunk"
		and type(update.content) == "table"
		and update.content.type == "text"
	then
		return update.content.text
	end
	return nil
end

local function stream_text(value)
	if type(value) ~= "table" then
		return nil
	end
	if value.type == "assistant" and type(value.message) == "table" and type(value.message.content) == "table" then
		local values = {}
		for _, content in ipairs(value.message.content) do
			if type(content) == "table" and content.type == "text" and type(content.text) == "string" then
				table.insert(values, content.text)
			end
		end
		return #values > 0 and table.concat(values) or nil
	end
	return nil
end

function M.supports(provider_name)
	return M.profiles[provider_name] ~= nil
end

function M.mode(provider_name)
	local _, value = profile(provider_name)
	return value.mode
end

function M.can_resume(provider_name)
	local _, value = profile(provider_name)
	return value.resume ~= false
end

function M.catalog(opts)
	if type(opts) ~= "table" or type(opts.cwd) ~= "string" then
		fail("catalog requires cwd and user_confirmed settings")
	end
	if type(opts.user_confirmed) ~= "table" then
		fail("catalog user_confirmed must be a table")
	end
	local cwd = directory(opts.cwd)
	local result = {}
	for name, value in pairs(M.profiles) do
		local record = { provider = name, mode = value.mode, available = false }
		if vim.fn.executable(value.executable) ~= 1 then
			record.reason = value.executable .. " is unavailable"
		elseif opts.user_confirmed[name] ~= true then
			record.reason = name .. " requires explicit providers." .. name .. ".user_confirmed opt-in"
		else
			local ok, adapter = pcall(require, value.adapter)
			local probe
			if ok and type(adapter.probe) == "function" then
				local probe_ok, result = pcall(adapter.probe, probe_options(value, cwd))
				if probe_ok then
					probe = result
				end
			end
			if type(probe) ~= "table" or not probe.available then
				record.reason = (type(probe) == "table" and probe.reason) or "provider probe failed"
			elseif probe.supported == false then
				record.reason = "installed version is outside Gator's supported range"
			elseif not profile_supported(value, probe) then
				record.reason = "installed CLI does not expose the required managed-session contract"
			else
				record.available = true
				record.authentication = "user_confirmed"
			end
		end
		table.insert(result, record)
	end
	table.sort(result, function(left, right)
		return left.provider < right.provider
	end)
	return result
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (opts.spawn ~= nil and type(opts.spawn) ~= "function") then
		fail("new requires an optional spawn function")
	end
	return setmetatable({ spawn = opts.spawn or default_spawn, active = {}, sequence = 0 }, Manager)
end

function Manager:is_active(reference)
	local provider_name, value = profile(reference.provider)
	local current = session(reference, provider_name, value.mode)
	local run = self.active[provider_name .. "\0" .. current.id]
	return run ~= nil and run.active == true
end

function Manager:_write(run, value)
	local encoded = vim.json.encode(value) .. "\n"
	local ok, result = pcall(run.handle.write, run.handle, encoded)
	if not ok or result == false then
		fail("provider process write failed")
	end
end

function Manager:_request(run, method, params, done)
	run.sequence = run.sequence + 1
	local id = run.sequence
	run.pending[id] = done
	self:_write(run, { jsonrpc = "2.0", id = id, method = method, params = params })
end

function Manager:_session(run, value)
	if type(value) ~= "string" or value == "" then
		fail("provider returned an invalid session id")
	end
	if run.session then
		if run.session.id ~= value then
			fail("provider changed the active session id")
		end
		return run.session
	end
	local next_key = run.provider .. "\0" .. value
	if run.key ~= next_key then
		if self.active[run.key] == run then
			self.active[run.key] = nil
		end
		run.key = next_key
		self.active[next_key] = run
	end
	run.session = {
		provider = run.provider,
		id = value,
		owner = run.mode == "history" and "gator" or "provider",
		mode = run.mode,
	}
	run.on_session(vim.deepcopy(run.session))
	return run.session
end

function Manager:_permission(run, raw)
	local params = type(raw.params) == "table" and raw.params or {}
	local request_id = params.requestId or params.request_id or raw.id
	run.on_permission({
		provider = run.provider,
		session_id = run.session and run.session.id or nil,
		request_id = tostring(request_id),
		action = raw.method,
		details = vim.deepcopy(params),
	}, function(decision)
		if raw.id == nil then
			return false
		end
		local selected
		for _, option in ipairs(type(params.options) == "table" and params.options or {}) do
			if type(option) == "table" and type(option.optionId) == "string" then
				if decision == "approved" and (option.kind == "allow_once" or option.kind == "allow_always") then
					selected = option.optionId
				elseif decision == "denied" and (option.kind == "reject_once" or option.kind == "reject_always") then
					selected = option.optionId
				end
			end
		end
		local outcome = decision == "cancelled" and { outcome = "cancelled" }
			or selected and { outcome = "selected", optionId = selected }
			or { outcome = "cancelled" }
		self:_write(run, { jsonrpc = "2.0", id = raw.id, result = { outcome = outcome } })
		return true
	end)
end

function Manager:_acp_line(run, raw)
	if raw.id ~= nil and (raw.result ~= nil or raw.error ~= nil) then
		local done = run.pending[raw.id]
		if done then
			run.pending[raw.id] = nil
			done(raw.result, raw.error)
		end
		return
	end
	if raw.method == "session/update" then
		local params = raw.params or {}
		if type(params.sessionId) == "string" then
			self:_session(run, params.sessionId)
		end
		local value = acp_text(params)
		if value then
			run.on_event({ type = "text", text = redact.text(value) })
		else
			run.on_event({ type = "update", update = redact.value(vim.deepcopy(params.update or {})) })
		end
		return
	end
	if raw.id ~= nil and (raw.method == "session/request_permission" or raw.method == "request_permission") then
		self:_permission(run, raw)
		return
	end
	if raw.id ~= nil and raw.method then
		self:_write(run, {
			jsonrpc = "2.0",
			id = raw.id,
			error = { code = -32601, message = "Gator does not implement " .. raw.method },
		})
	end
end

function Manager:_stream_line(run, raw)
	if type(raw.session_id) == "string" then
		self:_session(run, raw.session_id)
	end
	local value = stream_text(raw)
	if value then
		run.on_event({ type = "text", text = redact.text(value) })
	elseif raw.type == "result" then
		run.on_event({ type = "complete", text = redact.text(type(raw.result) == "string" and raw.result or "") })
	end
end

function Manager:_feed(run, chunk)
	if type(chunk) ~= "string" or chunk == "" then
		return
	end
	run.buffer = run.buffer .. chunk
	while true do
		local ending = run.buffer:find("\n", 1, true)
		if not ending then
			return
		end
		local line = run.buffer:sub(1, ending - 1):gsub("\r$", "")
		run.buffer = run.buffer:sub(ending + 1)
		if line ~= "" then
			local ok, raw = pcall(vim.json.decode, line)
			if ok and type(raw) == "table" then
				if run.mode == "acp" then
					self:_acp_line(run, raw)
				elseif run.mode == "stream" then
					self:_stream_line(run, raw)
				end
			else
				run.on_event({ type = "error", text = "provider emitted invalid structured output" })
			end
		end
	end
end

function Manager:_spawn(run, argv)
	local ok, handle = pcall(self.spawn, argv, {
		cwd = run.cwd,
		stdout = function(_, chunk)
			if run.mode == "json" or run.mode == "history" then
				run.buffer = run.buffer .. (type(chunk) == "string" and chunk or "")
			else
				self:_feed(run, chunk)
			end
		end,
		stderr = function(_, chunk)
			if type(chunk) == "string" and chunk ~= "" then
				run.stderr = run.stderr .. chunk
			end
		end,
	}, function(result)
		if run.mode == "json" and run.buffer ~= "" then
			local ok_result, value = pcall(droid_stream.parse, run.buffer)
			if not ok_result then
				run.on_event({ type = "error", text = "Droid emitted an invalid result" })
			else
				self:_session(run, value.id)
				run.on_event({ type = value.is_error and "error" or "complete", text = value.text })
			end
		elseif run.mode == "history" and run.buffer ~= "" then
			run.on_event({ type = "complete", text = redact.text(run.buffer) })
		end
		run.active = false
		if self.active[run.key] == run then
			self.active[run.key] = nil
		end
		run.on_exit({ code = type(result) == "table" and result.code or 1, stderr = redact.text(run.stderr) })
	end)
	if not ok or (type(handle) ~= "table" and type(handle) ~= "userdata") or type(handle.kill) ~= "function" then
		fail("provider process could not start")
	end
	run.handle, run.active = handle, true
end

function Manager:_start_acp(run, prompt)
	self:_request(run, "initialize", {
		protocolVersion = 1,
		clientCapabilities = vim.empty_dict(),
		clientInfo = { name = "gator", version = "1" },
	}, function(_, err)
		if err then
			run.on_event({ type = "error", text = "ACP initialization failed" })
			return
		end
		local method = run.session and "session/load" or "session/new"
		local params = run.session and { sessionId = run.session.id, cwd = run.cwd, mcpServers = {} }
			or { cwd = run.cwd, mcpServers = {} }
		self:_request(run, method, params, function(result, session_err)
			if session_err or type(result) ~= "table" or type(result.sessionId) ~= "string" then
				run.on_event({ type = "error", text = "ACP session " .. method .. " failed" })
				return
			end
			self:_session(run, result.sessionId)
			if prompt and prompt ~= "" then
				self:send(run.session, prompt)
			end
		end)
	end)
end

function Manager:open(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	local provider_name, value = profile(opts.provider)
	local mode, cwd = value.mode, directory(opts.cwd)
	local current = session(opts.session, provider_name, mode)
	if current and value.resume == false then
		fail(provider_name .. " does not document managed-session resume")
	end
	local task_id = text(opts.task_id, "task_id")
	local history = opts.history
	if mode == "history" then
		history = text(history, "history")
	end
	local key = provider_name .. "\0" .. (current and current.id or task_id)
	if self.active[key] then
		local active = self.active[key]
		if opts.prompt and opts.prompt ~= "" and active.session then
			self:send(active.session, opts.prompt)
		end
		return active
	end
	local run = {
		key = key,
		provider = provider_name,
		mode = mode,
		cwd = cwd,
		session = current,
		history = history,
		buffer = "",
		stderr = "",
		pending = {},
		sequence = 0,
		on_session = callback(opts.on_session, "on_session"),
		on_event = callback(opts.on_event, "on_event"),
		on_permission = callback(opts.on_permission, "on_permission"),
		on_exit = callback(opts.on_exit, "on_exit"),
	}
	self.active[key] = run
	if mode == "acp" then
		self:_spawn(run, profile_command(provider_name, value, current, "", history, cwd))
		self:_start_acp(run, opts.prompt)
	elseif opts.prompt and opts.prompt ~= "" then
		self:_spawn(run, profile_command(provider_name, value, current, opts.prompt, history, cwd))
		if mode == "stream" and provider_name == "amp" then
			self:_write(run, {
				type = "user",
				message = { role = "user", content = { { type = "text", text = opts.prompt } } },
			})
		elseif mode == "history" then
			self:_session(run, history)
		end
	else
		run.active = false
	end
	return run
end

function Manager:send(reference, prompt)
	local provider_name, value = profile(reference.provider)
	local current = session(reference, provider_name, value.mode)
	prompt = text(prompt, "prompt")
	local key = provider_name .. "\0" .. current.id
	local run = self.active[key]
	if value.mode == "acp" then
		if not run or not run.active then
			fail("ACP session is not active; reopen it before prompting")
		end
		self:_request(run, "session/prompt", {
			sessionId = current.id,
			prompt = { { type = "text", text = prompt } },
		}, function(_, err)
			if err then
				run.on_event({ type = "error", text = "ACP prompt failed" })
			end
		end)
		return true
	end
	if run and run.active then
		if value.mode == "stream" and provider_name == "amp" then
			self:_write(
				run,
				{ type = "user", message = { role = "user", content = { { type = "text", text = prompt } } } }
			)
			return true
		end
		fail("provider session is already running")
	end
	fail("managed session is inactive; reopen it with its workspace before prompting")
end

function Manager:cancel(reference)
	local provider_name, value = profile(reference.provider)
	local current = session(reference, provider_name, value.mode)
	local run = self.active[provider_name .. "\0" .. current.id]
	if not run or not run.active then
		return false
	end
	if value.mode == "acp" then
		self:_write(run, { jsonrpc = "2.0", method = "session/cancel", params = { sessionId = current.id } })
	else
		pcall(run.handle.kill, run.handle, 15)
	end
	return true
end

return M
