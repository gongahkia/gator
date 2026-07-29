local redact = require("gator.policy.redact")

local M = { api_version = 1 }
local Bridge = {}

Bridge.__index = Bridge

local supported = { codex = true, pi = true }

local function fail(message)
	error("Gator native terminal bridge: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function provider(value)
	value = text(value, "provider")
	if not supported[value] then
		fail("provider is unavailable: " .. value)
	end
	return value
end

local function session(value, provider_name)
	if type(value) ~= "table" or value.provider ~= provider_name or value.owner ~= "provider" then
		fail("session must remain provider-owned by " .. provider_name)
	end
	return { provider = provider_name, id = text(value.id, "session.id"), owner = "provider" }
end

local function workspace(value)
	value = text(value, "workspace")
	local resolved = vim.uv.fs_realpath(value)
	if not resolved or vim.fn.isdirectory(resolved) ~= 1 then
		fail("workspace must resolve to a directory")
	end
	return resolved
end

local function uuid()
	local hex = vim.fn.sha256(tostring(vim.uv.hrtime()) .. ":" .. tostring(math.random())):sub(1, 32)
	return table.concat({
		hex:sub(1, 8),
		hex:sub(9, 12),
		"4" .. hex:sub(14, 16),
		"8" .. hex:sub(18, 20),
		hex:sub(21, 32),
	}, "-")
end

local function command(executables, provider_name)
	return text(executables[provider_name] or provider_name, provider_name .. " executable")
end

local function rpc_client(opts, initialized, callback)
	local process, buffer, sequence, pending, closed = nil, "", 0, {}, false
	local function dispatch(fn)
		if vim.in_fast_event() then
			vim.schedule(fn)
		else
			fn()
		end
	end
	local function finish(result, reason)
		if closed then
			return
		end
		closed = true
		if process and type(process.kill) == "function" then
			pcall(process.kill, process, 15)
		end
		callback(result, reason and redact.text(reason) or nil)
	end
	local function request(method, params, done)
		sequence = sequence + 1
		local id = sequence
		pending[id] = done
		local ok, detail =
			pcall(process.write, process, vim.json.encode({ id = id, method = method, params = params }) .. "\n")
		if not ok or detail == false then
			pending[id] = nil
			finish(nil, "provider RPC write failed")
		end
	end
	local function notify(method, params)
		local ok, detail = pcall(process.write, process, vim.json.encode({ method = method, params = params }) .. "\n")
		if not ok or detail == false then
			finish(nil, "provider RPC notification could not be written")
			return false
		end
		return true
	end
	local function feed(chunk)
		if closed or type(chunk) ~= "string" or chunk == "" then
			return
		end
		buffer = buffer .. chunk
		while true do
			local ending = buffer:find("\n", 1, true)
			if not ending then
				return
			end
			local line = buffer:sub(1, ending - 1):gsub("\r$", "")
			buffer = buffer:sub(ending + 1)
			if line ~= "" then
				local ok, message = pcall(vim.json.decode, line)
				if ok and type(message) == "table" and message.id ~= nil then
					local done = pending[message.id]
					if done then
						pending[message.id] = nil
						done(message.result, message.error)
					end
				end
			end
		end
	end
	local ok, value = pcall(opts.spawn, opts.command, {
		cwd = opts.cwd,
		text = true,
		stdout = function(_, data)
			if data and data ~= "" then
				dispatch(function()
					feed(data)
				end)
			end
		end,
		stderr = function() end,
	}, function(result)
		dispatch(function()
			if not closed then
				finish(nil, "provider RPC exited " .. tostring(result and result.code or "before initialization"))
			end
		end)
	end)
	if not ok or (type(value) ~= "table" and type(value) ~= "userdata") or type(value.write) ~= "function" then
		finish(nil, "provider RPC could not start")
		return
	end
	process = value
	initialized(request, notify, finish)
	vim.defer_fn(function()
		dispatch(function()
			finish(nil, "provider RPC initialization timed out")
		end)
	end, 10000)
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("options must be a table")
	end
	for key in pairs(opts) do
		if key ~= "spawn" and key ~= "executables" and key ~= "uuid" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if opts.spawn ~= nil and type(opts.spawn) ~= "function" then
		fail("spawn must be a function")
	end
	if opts.executables ~= nil and type(opts.executables) ~= "table" then
		fail("executables must be a table")
	end
	if opts.uuid ~= nil and type(opts.uuid) ~= "function" then
		fail("uuid must be a function")
	end
	return setmetatable({
		spawn = opts.spawn or vim.system,
		executables = vim.deepcopy(opts.executables or {}),
		uuid = opts.uuid or uuid,
	}, Bridge)
end

function M.supports(provider_name)
	return supported[provider_name] == true
end

function Bridge:start(opts, callback)
	if type(opts) ~= "table" or type(callback) ~= "function" then
		fail("start requires options and callback")
	end
	local provider_name = provider(opts.provider)
	local cwd, prompt = workspace(opts.cwd), text(opts.prompt, "prompt")
	local executable = command(self.executables, provider_name)
	if provider_name == "pi" then
		local id = text(self.uuid(), "generated session id")
		callback({
			session = { provider = "pi", id = id, owner = "provider" },
			command = { executable, "--session-id", id, prompt },
		})
		return
	end
	if provider_name == "codex" then
		rpc_client(
			{ spawn = self.spawn, command = { executable, "app-server" }, cwd = cwd },
			function(request, notify, finish)
				request(
					"initialize",
					{ clientInfo = { name = "gator", title = "Gator", version = "1" } },
					function(_, err)
						if err then
							finish(nil, "Codex app-server initialization failed")
							return
						end
						if not notify("initialized", vim.empty_dict()) then
							return
						end
						request("thread/start", { cwd = cwd, ephemeral = false }, function(result, start_err)
							local id = result and result.thread and result.thread.id
							if start_err or type(id) ~= "string" or id == "" then
								finish(nil, "Codex thread creation failed")
								return
							end
							finish({
								session = { provider = "codex", id = id, owner = "provider" },
								command = { executable, "resume", id, prompt },
							})
						end)
					end
				)
			end,
			callback
		)
		return
	end
	fail("provider is unavailable: " .. provider_name)
end

function Bridge:resume(opts, callback)
	if type(opts) ~= "table" or type(callback) ~= "function" then
		fail("resume requires options and callback")
	end
	local provider_name = provider(opts.provider)
	local value = session(opts.session, provider_name)
	local executable = command(self.executables, provider_name)
	local argv
	if provider_name == "pi" then
		argv = { executable, "--session", value.id }
	else
		argv = { executable, "resume", value.id }
	end
	callback({ session = value, command = argv })
end

return M
