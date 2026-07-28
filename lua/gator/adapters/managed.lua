local redact = require("gator.policy.redact")
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
		resume = "dynamic",
	},
	cursor = { mode = "stream", executable = "cursor-agent", adapter = "gator.adapters.cursor" },
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

local function profile(name, profiles)
	name = text(name, "provider")
	local value = (profiles or M.profiles)[name]
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
	if value.capabilities ~= nil then
		if
			type(value.capabilities) ~= "table" or (vim.islist(value.capabilities) and next(value.capabilities) ~= nil)
		then
			fail("session capabilities must be an object")
		end
		for key, enabled in pairs(value.capabilities) do
			if type(key) ~= "string" or type(enabled) ~= "boolean" then
				fail("session capabilities must map names to booleans")
			end
		end
	end
	return {
		provider = provider_name,
		id = value.id,
		owner = value.owner,
		mode = mode,
		capabilities = value.capabilities and vim.deepcopy(value.capabilities) or nil,
	}
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

local function acp_phase(params)
	local update = type(params) == "table" and params.update
	if type(update) ~= "table" then
		return nil
	end
	local phases = {
		agent_thought_chunk = "thinking",
		tool_call = "using a tool",
		tool_call_update = "using a tool",
	}
	return phases[update.sessionUpdate]
end

local function acp_capabilities(value)
	local source = type(value) == "table" and value.agentCapabilities or nil
	if type(source) ~= "table" then
		return { loadSession = false, sessionResume = false, sessionList = false, sessionUsage = false }
	end
	return {
		loadSession = source.loadSession == true,
		sessionResume = source.sessionResume == true or source.resumeSession == true,
		sessionList = source.sessionList == true or source.listSessions == true,
		sessionUsage = source.sessionUsage == true or source.usage == true,
	}
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
	if
		opts.commands ~= nil
		and (type(opts.commands) ~= "table" or (vim.islist(opts.commands) and next(opts.commands) ~= nil))
	then
		fail("catalog commands must be an object")
	end
	local cwd = directory(opts.cwd)
	local profiles = vim.deepcopy(M.profiles)
	for name, configured in pairs(opts.commands or {}) do
		if type(name) ~= "string" or not name:match("^[a-z][a-z0-9_-]*$") then
			fail("configured ACP provider name is invalid")
		end
		if
			type(configured) ~= "table"
			or type(configured.argv) ~= "table"
			or not vim.islist(configured.argv)
			or #configured.argv == 0
			or type(configured.argv[1]) ~= "string"
			or configured.argv[1] == ""
		then
			fail("configured ACP command is invalid")
		end
		profiles[name] = {
			mode = "acp",
			executable = configured.argv[1],
			command = vim.deepcopy(configured.argv),
			configured = true,
		}
	end
	local result = {}
	for name, value in pairs(profiles) do
		local resume_mode = name == "copilot" and "dynamic_acp_then_terminal" or "managed"
		local record = {
			provider = name,
			mode = value.mode,
			available = false,
			readiness_state = "indeterminate",
			resume_mode = resume_mode,
		}
		if vim.fn.executable(value.executable) ~= 1 then
			record.reason = value.executable .. " is unavailable"
		elseif value.configured then
			record.available = true
			record.authentication = "configured"
			record.readiness_state = "configured"
			record.readiness_signals =
				{ "explicit ACP command", "executable detected", "capabilities negotiated at launch" }
		elseif opts.user_confirmed[name] ~= true then
			record.readiness_state = "detected"
			record.readiness_signals = { "CLI executable detected" }
			record.reason = name .. " is detected; explicit providers." .. name .. ".user_confirmed opt-in is required"
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
				record.readiness_state = "user_confirmed"
				record.readiness_signals = { "CLI contract detected", "user-confirmed configuration" }
			end
			if type(probe) == "table" then
				if type(probe.version) == "table" and #probe.version == 3 then
					record.version = table.concat(probe.version, ".")
				elseif type(probe.version) == "string" then
					record.version = probe.version
				end
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
	if
		type(opts) ~= "table"
		or (opts.spawn ~= nil and type(opts.spawn) ~= "function")
		or (opts.shutdown ~= nil and type(opts.shutdown) ~= "boolean")
		or (opts.commands ~= nil and type(opts.commands) ~= "table")
	then
		fail("new requires optional spawn and shutdown settings")
	end
	for key in pairs(opts) do
		if key ~= "spawn" and key ~= "shutdown" and key ~= "commands" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local value = setmetatable({
		spawn = opts.spawn or default_spawn,
		active = {},
		sequence = 0,
		profiles = vim.deepcopy(M.profiles),
		configured = {},
	}, Manager)
	value:configure({ commands = opts.commands or {} })
	if opts.shutdown then
		local group = vim.api.nvim_create_augroup("GatorManagedSessions", { clear = true })
		vim.api.nvim_create_autocmd("VimLeavePre", {
			group = group,
			once = true,
			callback = function()
				value:shutdown()
			end,
		})
	end
	return value
end

function Manager:configure(opts)
	if type(opts) ~= "table" or (opts.commands ~= nil and type(opts.commands) ~= "table") then
		fail("configure requires optional command settings")
	end
	for name in pairs(self.configured) do
		self.profiles[name] = nil
	end
	self.configured = {}
	for name, configured in pairs(opts.commands or {}) do
		if type(name) ~= "string" or not name:match("^[a-z][a-z0-9_-]*$") then
			fail("configured ACP provider name is invalid")
		end
		if
			type(configured) ~= "table"
			or type(configured.argv) ~= "table"
			or not vim.islist(configured.argv)
			or #configured.argv == 0
			or type(configured.argv[1]) ~= "string"
			or configured.argv[1] == ""
		then
			fail("configured ACP command is invalid")
		end
		self.profiles[name] = {
			mode = "acp",
			executable = configured.argv[1],
			command = command(configured.argv),
			configured = true,
		}
		self.configured[name] = true
	end
	return true
end

function Manager:profile(name)
	return profile(name, self.profiles)
end

function Manager:can_resume(provider_name, reference)
	local _, value = self:profile(provider_name)
	if value.mode ~= "acp" then
		return value.resume ~= false
	end
	if reference and type(reference.capabilities) == "table" then
		return reference.capabilities.loadSession == true or reference.capabilities.sessionResume == true
	end
	return true
end

function Manager:is_active(reference)
	local provider_name, value = self:profile(reference.provider)
	local current = session(reference, provider_name, value.mode)
	local run = self.active[provider_name .. "\0" .. current.id]
	return run ~= nil and run.active == true and run.stopping ~= true
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

function Manager:_terminate(run)
	if run.terminated then
		return false
	end
	run.terminated, run.active = true, false
	if self.active[run.key] == run then
		self.active[run.key] = nil
	end
	pcall(run.handle.kill, run.handle, 15)
	return true
end

function Manager:_copilot_fallback(run, reason)
	if not run.has_resume_fallback then
		run.on_event({ type = "error", text = reason })
		self:_terminate(run)
		return false
	end
	run.fallback = true
	self:_terminate(run)
	run.on_resume_fallback({
		provider = run.provider,
		session = vim.deepcopy(run.session),
		reason = reason,
	})
	return true
end

function Manager:_session(run, value)
	if type(value) ~= "string" or value == "" then
		fail("provider returned an invalid session id")
	end
	if run.session then
		if run.session.id ~= value then
			fail("provider changed the active session id")
		end
		local announce = run.restoring == true
		if run.capabilities then
			announce = announce or not vim.deep_equal(run.session.capabilities, run.capabilities)
			run.session.capabilities = vim.deepcopy(run.capabilities)
		end
		run.restoring = false
		if announce then
			run.on_session(vim.deepcopy(run.session))
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
		capabilities = run.capabilities,
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
			local phase = acp_phase(params)
			if phase then
				run.on_event({ type = "phase", phase = phase })
			else
				run.on_event({ type = "update", update = redact.value(vim.deepcopy(params.update or {})) })
			end
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
			if run.mode == "history" then
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
		if run.mode == "history" and run.buffer ~= "" then
			run.on_event({ type = "complete", text = redact.text(run.buffer) })
		end
		run.active, run.terminated = false, true
		if self.active[run.key] == run then
			self.active[run.key] = nil
		end
		run.on_exit({
			code = type(result) == "table" and result.code or 1,
			stderr = redact.text(run.stderr),
			fallback = run.fallback == true,
			stopped = run.stopped == true,
		})
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
	}, function(initialized, err)
		if err then
			run.on_event({ type = "error", text = "ACP initialization failed" })
			return
		end
		run.capabilities = acp_capabilities(initialized)
		local method = "session/new"
		if run.session then
			if run.capabilities.loadSession then
				method = "session/load"
			elseif run.capabilities.sessionResume then
				method = "session/resume"
			elseif run.provider == "copilot" then
				self:_copilot_fallback(run, "Copilot ACP did not advertise session/load")
				return
			else
				run.on_event({ type = "error", text = "ACP agent did not advertise session restore capability" })
				self:_terminate(run)
				return
			end
		end
		local params = run.session and { sessionId = run.session.id, cwd = run.cwd, mcpServers = {} }
			or { cwd = run.cwd, mcpServers = {} }
		self:_request(run, method, params, function(result, session_err)
			if session_err or type(result) ~= "table" or type(result.sessionId) ~= "string" then
				if run.provider == "copilot" and method == "session/load" then
					self:_copilot_fallback(run, "Copilot ACP session/load was rejected")
					return
				end
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

function Manager:list(reference, callback)
	if type(callback) ~= "function" then
		fail("session list requires a callback")
	end
	local provider_name, value = self:profile(reference.provider)
	if value.mode ~= "acp" then
		fail("session list is available only for ACP providers")
	end
	local current = session(reference, provider_name, value.mode)
	local run = self.active[provider_name .. "\0" .. current.id]
	if not run or not run.active or not run.capabilities or not run.capabilities.sessionList then
		fail("ACP agent did not advertise session/list")
	end
	self:_request(run, "session/list", { cwd = run.cwd }, function(result, err)
		if err then
			callback(nil, "ACP session/list failed")
			return
		end
		callback(vim.deepcopy(result), nil)
	end)
	return true
end

function Manager:open(opts)
	if type(opts) ~= "table" then
		fail("open requires options")
	end
	local provider_name, value = self:profile(opts.provider)
	local mode, cwd = value.mode, directory(opts.cwd)
	local current = session(opts.session, provider_name, mode)
	if current and value.resume == false then
		fail(provider_name .. " does not document managed-session resume")
	end
	local run_id = text(opts.run_id, "run_id")
	local history = opts.history
	if mode == "history" then
		history = text(history, "history")
	end
	local key = provider_name .. "\0" .. (current and current.id or run_id)
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
		restoring = current ~= nil,
		history = history,
		buffer = "",
		stderr = "",
		pending = {},
		sequence = 0,
		on_session = callback(opts.on_session, "on_session"),
		on_event = callback(opts.on_event, "on_event"),
		on_permission = callback(opts.on_permission, "on_permission"),
		on_resume_fallback = callback(opts.on_resume_fallback, "on_resume_fallback"),
		has_resume_fallback = type(opts.on_resume_fallback) == "function",
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
	local provider_name, value = self:profile(reference.provider)
	local current = session(reference, provider_name, value.mode)
	prompt = text(prompt, "prompt")
	local key = provider_name .. "\0" .. current.id
	local run = self.active[key]
	if value.mode == "acp" then
		if not run or not run.active or run.stopping then
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
	local provider_name, value = self:profile(reference.provider)
	local current = session(reference, provider_name, value.mode)
	local run = self.active[provider_name .. "\0" .. current.id]
	if not run or not run.active or run.stopping then
		return false
	end
	if value.mode == "acp" then
		self:_write(run, { jsonrpc = "2.0", method = "session/cancel", params = { sessionId = current.id } })
	else
		pcall(run.handle.kill, run.handle, 15)
	end
	return true
end

function Manager:stop(reference)
	local provider_name, value = self:profile(reference.provider)
	local current = session(reference, provider_name, value.mode)
	local run = self.active[provider_name .. "\0" .. current.id]
	if not run or not run.active or run.stopping then
		return false
	end
	run.stopping, run.stopped = true, true
	return self:_terminate(run)
end

function Manager:shutdown()
	local references = {}
	for _, run in pairs(self.active) do
		if run.active and run.session then
			table.insert(references, vim.deepcopy(run.session))
		end
	end
	table.sort(references, function(left, right)
		return left.provider == right.provider and left.id < right.id or left.provider < right.provider
	end)
	local stopped = 0
	for _, reference in ipairs(references) do
		if self:stop(reference) then
			stopped = stopped + 1
		end
	end
	return stopped
end

return M
