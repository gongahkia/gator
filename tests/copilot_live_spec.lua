if vim.env.GATOR_LIVE_COPILOT ~= "1" then
	return
end

local managed = require("gator").module("adapters").managed

local version = vim.system({ "copilot", "version" }, { text = true }):wait()
assert(
	version.code == 0 and (version.stdout or ""):find("GitHub Copilot CLI", 1, true),
	"protected Copilot verification requires the CLI version command"
)
local help = vim.system({ "copilot", "--help" }, { text = true }):wait()
local output = help.stdout or ""
assert(help.code == 0, "protected Copilot verification requires the CLI")
assert(output:find("--acp", 1, true), "protected Copilot verification requires ACP support")
assert(output:find("--resume", 1, true), "protected Copilot verification requires native session resume")
assert(output:find("--available-tools", 1, true), "protected Copilot verification requires native tool filters")

if vim.env.GATOR_LIVE_COPILOT_AUTH ~= "1" then
	return
end

local workspace = vim.fn.tempname()
assert(vim.fn.mkdir(workspace, "p") == 1, "authenticated Copilot verification requires a temporary workspace")
local result = vim.system({
	"copilot",
	"--prompt",
	"Reply exactly: gator-live-e2e",
	"--available-tools",
	"view",
	"glob",
	"grep",
	"--no-custom-instructions",
	"--silent",
	"--stream",
	"off",
}, { cwd = workspace, text = true }):wait()
assert(result.code == 0, "authenticated Copilot verification requires a successful headless run")
assert(vim.trim(result.stdout or "") == "gator-live-e2e", "Copilot E2E must preserve its exact response")

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

local first_frames, first_events, first_session, first_exit = {}, {}, nil, nil
local first = managed.new({ spawn = bridge_spawn(first_frames) })
first:open({
	provider = "copilot",
	cwd = workspace,
	task_id = "copilot-live-bridge",
	prompt = "Reply exactly: gator-live-e2e. Do not use tools or edit files.",
	on_session = function(value)
		first_session = value
	end,
	on_event = function(event)
		table.insert(first_events, event)
	end,
	on_exit = function(value)
		first_exit = value
	end,
})
wait_for(function()
	return first_session ~= nil and has_event(first_events, "complete")
end, "authenticated Copilot bridge requires a created session and completed prompt")
assert(first:stop(first_session), "authenticated Copilot bridge requires managed stdio shutdown")
wait_for(function()
	return first_exit ~= nil
end, "authenticated Copilot bridge requires first child termination")

local resumed_frames, resumed_events, resumed_exit, fallback = {}, {}, nil, nil
local resumed = managed.new({ spawn = bridge_spawn(resumed_frames) })
resumed:open({
	provider = "copilot",
	cwd = workspace,
	task_id = "copilot-live-bridge",
	session = first_session,
	prompt = "Reply exactly: gator-live-e2e. Do not use tools or edit files.",
	on_event = function(event)
		table.insert(resumed_events, event)
	end,
	on_resume_fallback = function(value)
		fallback = value
	end,
	on_exit = function(value)
		resumed_exit = value
	end,
})
wait_for(function()
	return fallback ~= nil or has_event(resumed_events, "complete")
end, "authenticated Copilot bridge requires ACP load or explicit terminal fallback")
if fallback then
	assert(
		not has_method(resumed_frames, "session/load") and fallback.session.id == first_session.id,
		"Copilot without loadSession capability must fall back before sending an ACP load request"
	)
	local terminal = vim.system({
		"copilot",
		"--resume",
		first_session.id,
		"--prompt",
		"Reply exactly: gator-live-e2e",
		"--available-tools",
		"view",
		"glob",
		"grep",
		"--no-custom-instructions",
		"--silent",
		"--stream",
		"off",
	}, { cwd = workspace, text = true }):wait()
	assert(
		terminal.code == 0 and vim.trim(terminal.stdout or "") == "gator-live-e2e",
		"Copilot terminal fallback must resume the ACP-created provider session"
	)
else
	assert(has_method(resumed_frames, "session/load"), "Copilot with loadSession capability must reattach through ACP")
	assert(resumed:stop(first_session), "authenticated Copilot ACP reattach requires managed stdio shutdown")
	wait_for(function()
		return resumed_exit ~= nil
	end, "authenticated Copilot bridge requires resumed child termination")
end
assert(
	has_method(first_frames, "initialize")
		and has_method(first_frames, "session/new")
		and has_method(first_frames, "session/prompt"),
	"authenticated Copilot bridge must initialize, create, and prompt through ACP"
)
vim.fn.delete(workspace, "d")
