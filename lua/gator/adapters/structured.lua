local redact = require("gator.policy.redact")
local codex_permission = require("gator.adapters.codex_permission")

local M = {}
local Manager = {}
Manager.__index = Manager
M.startup_timeout_ms = 15000

local function fail(message)
	error("Gator structured transport: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function codex_policy(value)
	if value == nil then
		return nil
	end
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail("codex_policy must be an object")
	end
	for key in pairs(value) do
		if key ~= "sandbox" and key ~= "approval_policy" then
			fail("codex_policy contains unsupported field: " .. tostring(key))
		end
	end
	if (value.sandbox ~= "read-only" and value.sandbox ~= "workspace-write") or value.approval_policy ~= "on-request" then
		fail("codex_policy must use a supported sandbox and on-request approvals")
	end
	return vim.deepcopy(value)
end

local function event_text(value)
	if type(value) ~= "table" then
		return nil
	end
	if value.type == "message_update" and type(value.assistantMessageEvent) == "table" then
		local update = value.assistantMessageEvent
		return update.type == "text_delta" and type(update.delta) == "string" and update.delta or nil
	end
	if type(value.delta) == "string" then
		return value.delta
	end
	if type(value.text) == "string" then
		return value.text
	end
	return nil
end

local function pi_usage(value)
	local tokens = type(value) == "table" and value.tokens
	if type(tokens) ~= "table" then
		return nil
	end
	if type(tokens.total) ~= "number" then
		return nil
	end
	return {
		state = "reported",
		input_tokens = type(tokens.input) == "number" and tokens.input or nil,
		output_tokens = type(tokens.output) == "number" and tokens.output or nil,
		total_tokens = tokens.total,
	}
end

local function codex_usage(value)
	if type(value) ~= "table" then
		return nil
	end
	local usage = value.usage or value.tokenUsage
	if type(usage) ~= "table" then
		return nil
	end
	local total = usage.total_tokens or usage.totalTokens
	if type(total) ~= "number" then
		return nil
	end
	return {
		state = "reported",
		input_tokens = usage.input_tokens or usage.inputTokens,
		output_tokens = usage.output_tokens or usage.outputTokens,
		total_tokens = total,
	}
end

function M.supports(provider)
	return provider == "pi" or provider == "codex"
end

function M.new(opts)
	opts = opts or {}
	if
		type(opts) ~= "table"
		or (opts.spawn ~= nil and type(opts.spawn) ~= "function")
		or (
			opts.startup_timeout_ms ~= nil
			and (
				type(opts.startup_timeout_ms) ~= "number"
				or opts.startup_timeout_ms < 0
				or opts.startup_timeout_ms % 1 ~= 0
			)
		)
	then
		fail("new requires optional spawn and startup_timeout_ms settings")
	end
	return setmetatable({
		spawn = opts.spawn or vim.system,
		startup_timeout_ms = opts.startup_timeout_ms or M.startup_timeout_ms,
		active = {},
		sequence = 0,
	}, Manager)
end

function Manager:open(opts)
	if type(opts) ~= "table" or not M.supports(opts.provider) then
		fail("open requires a structured provider")
	end
	local provider, cwd = opts.provider, text(opts.cwd, "cwd")
	local launch_policy = codex_policy(opts.codex_policy)
	if launch_policy and provider ~= "codex" then
		fail("codex_policy is available only for Codex")
	end
	local prompt = opts.prompt
	if prompt ~= nil then
		prompt = text(prompt, "prompt")
	end
	local operation = opts.operation or "start"
	if operation ~= "start" and operation ~= "resume" and operation ~= "fork" then
		fail("operation must be start, resume, or fork")
	end
	local existing = opts.session
	if operation == "start" then
		if existing ~= nil then
			fail("new structured sessions cannot provide an existing session")
		end
		if not prompt then
			fail("new structured sessions require a prompt")
		end
	elseif type(existing) ~= "table" or type(existing.id) ~= "string" or existing.id == "" then
		fail(operation .. " requires a provider session")
	end
	for _, name in ipairs({ "on_session", "on_event", "on_usage", "on_exit", "on_approval" }) do
		if opts[name] ~= nil and type(opts[name]) ~= "function" then
			fail(name .. " must be a function")
		end
	end
	self.sequence = self.sequence + 1
	local id = provider .. "-" .. self.sequence
	local current = {
		id = id,
		provider = provider,
		buffer = "",
		busy = false,
		closed = false,
		session_id = nil,
		turn_id = nil,
		pending = {},
		permission = nil,
		run_id = opts.run_id or "run-structured",
		operation = operation,
		existing = existing and vim.deepcopy(existing) or nil,
	}
	self.active[id] = current
	local function dispatch(callback)
		if vim.in_fast_event() then
			vim.schedule(callback)
		else
			callback()
		end
	end
	local function notify(kind, value)
		if opts.on_event then
			opts.on_event(kind, value)
		end
	end
	local function stop_startup_timer()
		local timer = current.startup_timer
		current.startup_timer = nil
		if timer and not timer:is_closing() then
			timer:stop()
			timer:close()
		end
	end
	local function fail_startup(reason)
		if current.closed or current.startup_failed then
			return false
		end
		current.startup_failed = true
		stop_startup_timer()
		if reason then
			notify("error", reason)
		end
		if current.handle then
			pcall(current.handle.kill, current.handle, 15)
		end
		return false
	end
	local function usage(value)
		local result = provider == "pi" and pi_usage(value) or codex_usage(value)
		if result and opts.on_usage then
			opts.on_usage(result)
		end
	end
	local function write(value)
		if current.closed or not current.handle then
			return false
		end
		local ok, result = pcall(current.handle.write, current.handle, vim.json.encode(value) .. "\n")
		return ok and result ~= false
	end
	local send
	local function accept_session(value)
		if operation == "resume" and value.id ~= existing.id then
			notify("error", "provider resume returned a different session identity")
			current.closed = true
			self.active[id] = nil
			if current.handle then
				pcall(current.handle.kill, current.handle, 15)
			end
			return false
		end
		stop_startup_timer()
		current.session_id = value.id
		if opts.on_session then
			opts.on_session(value)
		end
		if prompt then
			send(prompt)
		end
		return true
	end
	local function pi_command(command, message)
		if write({ id = tostring(vim.uv.hrtime()), type = command, message = message }) then
			return true
		end
		if not current.session_id then
			return fail_startup("Pi startup request could not be written")
		end
		notify("error", "Pi request could not be written")
		return false
	end
	local function codex_request(method, params)
		current.request_id = (current.request_id or 0) + 1
		local request_id = current.request_id
		if not write({ id = request_id, method = method, params = params }) then
			if not current.session_id then
				return fail_startup("Codex startup request could not be written: " .. method)
			end
			notify("error", "Codex request could not be written: " .. method)
			return false
		end
		current.pending[request_id] = method
		return true
	end
	send = function(message)
		message = text(message, "prompt")
		if provider == "pi" then
			local body = { id = tostring(vim.uv.hrtime()), type = "prompt", message = message }
			if current.busy then
				body.streamingBehavior = "steer"
			end
			return write(body)
		end
		local input = { { type = "text", text = message } }
		if current.busy then
			if not current.turn_id then
				return false
			end
			return codex_request(
				"turn/steer",
				{ threadId = current.session_id, expectedTurnId = current.turn_id, input = input }
			)
		end
		return codex_request("turn/start", { threadId = current.session_id, input = input })
	end
	local function establish_permission_bridge()
		if provider ~= "codex" or current.permission then
			return
		end
		current.permission = codex_permission.new({
			respond = function(response)
				if not write({ id = response.id, result = response.result }) then
					error("Codex approval response could not be written")
				end
			end,
		})
	end
	local function approval(message)
		if not current.permission then
			return false
		end
		local ok, emitted = pcall(current.permission.receive, current.permission, {
			id = message.id,
			method = message.method,
			params = message.params,
		}, {
			run_id = current.run_id,
			session_id = current.session_id,
		})
		if not ok then
			notify("error", emitted)
			return true
		end
		if not emitted or not emitted.payload then
			return false
		end
		local function decide(decision)
			local resolved, result =
				pcall(current.permission.decide, current.permission, emitted.payload.request_id, decision)
			if not resolved then
				notify("error", result)
			end
			return resolved and result
		end
		if opts.on_approval then
			opts.on_approval({
				action = emitted.payload.action,
				details = vim.deepcopy(emitted.payload.details),
				command = type(message.params) == "table" and redact.text(message.params.command or "") or "",
			}, decide)
		else
			decide("cancelled")
		end
		return true
	end
	local function handle_message(message)
		if provider == "pi" then
			if
				message.type == "response"
				and message.command == "get_state"
				and message.success
				and type(message.data) == "table"
			then
				local session_id = message.data.sessionId
				if type(session_id) == "string" and session_id ~= "" then
					accept_session({
						id = session_id,
						path = message.data.sessionFile,
						resume_supported = true,
					})
				end
				return
			end
			if message.type == "agent_start" then
				current.busy = true
				notify("running")
			elseif message.type == "agent_settled" then
				current.busy = false
				notify("settled")
				pi_command("get_session_stats")
			elseif message.type == "response" and message.command == "get_session_stats" and message.success then
				usage(message.data)
			end
			local delta = event_text(message)
			if delta then
				notify("text", delta)
			end
			return
		end
		if message.id ~= nil and message.method then
			if approval(message) then
				return
			end
			write({
				id = message.id,
				error = { code = -32601, message = "Gator does not implement " .. tostring(message.method) },
			})
			notify("error", "Codex requested unsupported client action: " .. tostring(message.method))
			return
		end
		if message.id ~= nil and (message.result ~= nil or message.error ~= nil) then
			local requested = current.pending[message.id]
			current.pending[message.id] = nil
			if message.error then
				local reason =
					redact.text(type(message.error) == "table" and message.error.message or "Codex request failed")
				if not current.session_id then
					fail_startup(reason)
				else
					notify("error", reason)
				end
				return
			end
			if requested == "initialize" then
				current.initialized = true
				current.initialized_notification_sent = write({ method = "initialized", params = vim.empty_dict() })
				if not current.initialized_notification_sent then
					fail_startup("Codex initialized notification could not be written")
					return
				end
				local params = launch_policy
						and {
							cwd = cwd,
							approvalPolicy = launch_policy.approval_policy,
							sandbox = launch_policy.sandbox,
						}
					or { cwd = cwd }
				if current.operation == "start" then
					params.ephemeral = false
					codex_request("thread/start", params)
				elseif current.operation == "resume" then
					params.threadId = current.existing.id
					codex_request("thread/resume", params)
				else
					params.threadId = current.existing.id
					codex_request("thread/fork", params)
				end
				return
			end
			if
				(requested == "thread/start" or requested == "thread/resume" or requested == "thread/fork")
				and current.session_id == nil
			then
				local thread = message.result.thread or message.result
				local session_id = thread.id or thread.threadId
				if type(session_id) == "string" and session_id ~= "" then
					establish_permission_bridge()
					accept_session({ id = session_id, resume_supported = true })
				end
				return
			end
			if requested == "turn/start" or requested == "turn/steer" then
				local turn = message.result and message.result.turn
				if type(turn) == "table" and type(turn.id) == "string" then
					current.turn_id = turn.id
				end
			end
			return
		end
		local method = message.method
		if method == "turn/started" then
			current.busy = true
			local turn = type(message.params) == "table" and message.params.turn or nil
			if type(turn) == "table" and type(turn.id) == "string" then
				current.turn_id = turn.id
			end
			notify("running")
		elseif method == "turn/completed" or method == "turn/finished" then
			current.busy = false
			current.turn_id = nil
			notify("settled")
		end
		usage(message.params or message.result)
		local delta = event_text(message.params or message)
		if delta then
			notify("text", delta)
		end
	end
	local function feed(chunk)
		if current.closed or type(chunk) ~= "string" or chunk == "" then
			return
		end
		current.buffer = current.buffer .. chunk
		while true do
			local ending = current.buffer:find("\n", 1, true)
			if not ending then
				return
			end
			local line = current.buffer:sub(1, ending - 1):gsub("\r$", "")
			current.buffer = current.buffer:sub(ending + 1)
			if line ~= "" then
				local ok, message = pcall(vim.json.decode, line)
				if ok and type(message) == "table" then
					handle_message(message)
				end
			end
		end
	end
	local argv
	if provider == "pi" then
		argv = { "pi", "--mode", "rpc" }
		if operation == "start" then
			vim.list_extend(argv, { "--name", id })
		elseif operation == "resume" then
			vim.list_extend(argv, { "--session", existing.path or existing.id })
		else
			vim.list_extend(argv, { "--fork", existing.path or existing.id })
		end
	else
		argv = { "codex", "app-server" }
	end
	local handle = self.spawn(argv, {
		cwd = cwd,
		text = true,
		stdin = true,
		stdout = function(_, data)
			if data and data ~= "" then
				dispatch(function()
					feed(data)
				end)
			end
		end,
		stderr = function(_, data)
			if data and data ~= "" then
				dispatch(function()
					notify("error", redact.text(data))
				end)
			end
		end,
	}, function(result)
		dispatch(function()
			local stopped = current.stopped
			current.closed = true
			stop_startup_timer()
			self.active[id] = nil
			if opts.on_exit and not stopped then
				opts.on_exit({ code = result and result.code or 1 })
			end
		end)
	end)
	if type(handle) ~= "table" and type(handle) ~= "userdata" then
		self.active[id] = nil
		fail("provider process could not start")
	end
	current.handle = handle
	if self.startup_timeout_ms > 0 then
		local timer = vim.uv.new_timer()
		current.startup_timer = timer
		timer:start(
			self.startup_timeout_ms,
			0,
			vim.schedule_wrap(function()
				if current.startup_timer == timer then
					current.startup_timer = nil
					if not timer:is_closing() then
						timer:close()
					end
				end
				if not current.closed and not current.session_id then
					fail_startup(provider .. " startup timed out waiting for a provider session")
				end
			end)
		)
	end
	if provider == "pi" then
		pi_command("get_state")
	else
		codex_request("initialize", { clientInfo = { name = "gator", title = "Gator", version = "1" } })
	end
	return {
		id = id,
		send = send,
		cancel = function()
			if provider == "pi" then
				return pi_command("abort")
			end
			if not current.session_id or not current.turn_id then
				return false
			end
			return codex_request("turn/interrupt", { threadId = current.session_id, turnId = current.turn_id })
		end,
		stop = function()
			current.stopped = true
			current.closed = true
			stop_startup_timer()
			self.active[id] = nil
			return pcall(handle.kill, handle, 15)
		end,
	}
end

function Manager:resume(opts)
	if type(opts) ~= "table" then
		fail("resume requires options")
	end
	opts = vim.deepcopy(opts)
	opts.operation = "resume"
	return self:open(opts)
end

function Manager:fork(opts)
	if type(opts) ~= "table" then
		fail("fork requires options")
	end
	opts = vim.deepcopy(opts)
	opts.operation = "fork"
	return self:open(opts)
end

return M
