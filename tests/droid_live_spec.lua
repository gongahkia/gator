if vim.env.GATOR_LIVE_DROID ~= "1" then
	return
end

local droid = require("gator").module("adapters").droid
local stream = require("gator").module("adapters").droid_stream
local managed = require("gator").module("adapters").managed
local value = droid.probe()
assert(value.available and value.supported, "protected Droid verification requires the documented exec profile")
assert(
	value.capabilities.structured_output
		and value.capabilities.stream_jsonrpc
		and value.capabilities.session_resume
		and value.capabilities.session_fork
		and value.capabilities.autonomy,
	"protected Droid verification requires documented execution capabilities"
)

if vim.env.GATOR_LIVE_DROID_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Droid verification requires a temporary workspace")
local result = vim.system({
	"droid",
	"exec",
	"--cwd",
	workspace,
	"--output-format",
	"json",
	"Reply with a brief greeting.",
}, { cwd = workspace, text = true }):wait()
assert(result.code == 0, "authenticated Droid verification requires a successful read-only run")
assert(
	stream.parse(result.stdout or "").text ~= "",
	"authenticated Droid verification requires a documented JSON result"
)

local function bridge_spawn(frames)
	return function(argv, opts, done)
		local process = vim.system(argv, {
			cwd = opts.cwd,
			text = true,
			stdin = true,
			stdout = opts.stdout,
			stderr = opts.stderr,
		}, done)
		return {
			write = function(_, frame)
				table.insert(frames, vim.json.decode(frame))
				return process:write(frame)
			end,
			kill = function(_, signal)
				return process:kill(signal)
			end,
		}
	end
end

local function has_method(frames, method)
	for _, frame in ipairs(frames) do
		if frame.method == method then
			return true
		end
	end
	return false
end

local function has_event(events, kind)
	for _, event in ipairs(events) do
		if event.type == kind then
			return true
		end
	end
	return false
end

local function wait_for(predicate, message)
	assert(vim.wait(120000, predicate, 50), message)
end

local function run_bridge(session)
	local resumed = session ~= nil
	local frames, output, exited = {}, {}, nil
	local runtime = managed.new({ spawn = bridge_spawn(frames) })
	runtime:open({
		provider = "droid",
		cwd = workspace,
		task_id = "droid-live-bridge",
		session = session,
		prompt = "Reply exactly: gator-live-e2e. Do not use tools or edit files.",
		on_session = function(value)
			session = value
		end,
		on_event = function(event)
			table.insert(output, event)
		end,
		on_exit = function(value)
			exited = value
		end,
	})
	wait_for(function()
		return session ~= nil and has_event(output, "complete")
	end, "authenticated Droid bridge requires initialized session and completed prompt")
	assert(runtime:stop(session), "authenticated Droid bridge requires managed close")
	wait_for(function()
		return exited ~= nil
	end, "authenticated Droid bridge requires child termination after close")
	assert(
		has_method(frames, resumed and "droid.load_session" or "droid.initialize_session")
			and has_method(frames, "droid.add_user_message")
			and has_method(frames, "droid.close_session")
			and exited.stopped,
		"authenticated Droid bridge must initialize or load, prompt, close, and terminate deterministically"
	)
	return session
end

local first = run_bridge(nil)
local second = run_bridge(first)
assert(second.id == first.id, "authenticated Droid bridge must retain the provider session id across fresh processes")
vim.fn.delete(workspace, "d")
