local redact = require("gator.policy.redact")

local M = {}
local Manager = {}
Manager.__index = Manager

local function fail(message)
	error("Gator structured transport: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
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
	if type(opts) ~= "table" or (opts.spawn ~= nil and type(opts.spawn) ~= "function") then
		fail("new requires an optional spawn function")
	end
	return setmetatable({ spawn = opts.spawn or vim.system, active = {}, sequence = 0 }, Manager)
end

function Manager:open(opts)
	if type(opts) ~= "table" or not M.supports(opts.provider) then
		fail("open requires a structured provider")
	end
	local provider, cwd, prompt = opts.provider, text(opts.cwd, "cwd"), text(opts.prompt, "prompt")
	for _, name in ipairs({ "on_session", "on_event", "on_usage", "on_exit" }) do
		if opts[name] ~= nil and type(opts[name]) ~= "function" then
			fail(name .. " must be a function")
		end
	end
	self.sequence = self.sequence + 1
	local id = provider .. "-" .. self.sequence
	local current = { id = id, provider = provider, buffer = "", busy = false, closed = false, session_id = nil }
	self.active[id] = current
	local function notify(kind, value)
		if opts.on_event then
			opts.on_event(kind, value)
		end
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
	local function pi_command(command, message)
		return write({ id = tostring(vim.uv.hrtime()), type = command, message = message })
	end
	local function codex_request(method, params)
		current.request_id = (current.request_id or 0) + 1
		return write({ jsonrpc = "2.0", id = current.request_id, method = method, params = params })
	end
	local function send(message)
		message = text(message, "prompt")
		if provider == "pi" then
			local body = { id = tostring(vim.uv.hrtime()), type = "prompt", message = message }
			if current.busy then
				body.streamingBehavior = "steer"
			end
			return write(body)
		end
		return codex_request("turn/start", {
			threadId = current.session_id,
			input = { { type = "text", text = message } },
		})
	end
	local function handle_message(message)
		if provider == "pi" then
			if message.type == "response" and message.command == "get_state" and message.success and type(message.data) == "table" then
				current.session_id = message.data.sessionId
				if current.session_id and opts.on_session then
					opts.on_session({ id = current.session_id, resume_supported = false })
				end
				send(prompt)
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
		if message.id == 1 and message.result and not current.initialized then
			current.initialized = true
			write({ jsonrpc = "2.0", method = "initialized", params = vim.empty_dict() })
			codex_request("thread/start", { cwd = cwd, ephemeral = false })
			return
		end
		if message.id ~= nil and message.result and current.session_id == nil then
			local thread = message.result.thread or message.result
			local session_id = thread.id or thread.threadId
			if type(session_id) == "string" and session_id ~= "" then
				current.session_id = session_id
				if opts.on_session then
					opts.on_session({ id = session_id, resume_supported = true })
				end
				send(prompt)
			end
		end
		local method = message.method
		if method == "turn/started" then
			current.busy = true
			notify("running")
		elseif method == "turn/completed" or method == "turn/finished" then
			current.busy = false
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
	local argv = provider == "pi" and { "pi", "--mode", "rpc", "--name", id } or { "codex", "app-server", "--stdio" }
	local handle = self.spawn(argv, {
		cwd = cwd,
		text = true,
		stdin = true,
		stdout = function(_, data)
			feed(data)
		end,
		stderr = function(_, data)
			if data and data ~= "" then
				notify("error", redact.text(data))
			end
		end,
	}, function(result)
		current.closed = true
		self.active[id] = nil
		if opts.on_exit then
			opts.on_exit({ code = result and result.code or 1 })
		end
	end)
	if type(handle) ~= "table" and type(handle) ~= "userdata" then
		self.active[id] = nil
		fail("provider process could not start")
	end
	current.handle = handle
	if provider == "pi" then
		pi_command("get_state")
	else
		codex_request("initialize", { clientInfo = { name = "gator", version = "1" }, capabilities = vim.empty_dict() })
	end
	return {
		id = id,
		send = send,
		cancel = function()
			return provider == "pi" and pi_command("abort") or write({ jsonrpc = "2.0", method = "turn/interrupt", params = { threadId = current.session_id } })
		end,
		stop = function()
			current.closed = true
			self.active[id] = nil
			return pcall(handle.kill, handle, 15)
		end,
	}
end

return M
