local compat = require("gator.compat")
local coordinator = require("gator.coordinator")

local M = {
	_state = nil,
	_coordinator = nil,
	error = require("gator.error"),
	modules = coordinator.modules,
}

function M.module(name)
	if name == "coordinator" then
		return coordinator
	end
	if M._coordinator then
		return M._coordinator:module(name)
	end
	return coordinator.module(name)
end

function M.setup(opts)
	M._coordinator = coordinator.new(opts)
	M._state = M._coordinator:state()
	return M
end

function M.compatibility()
	return compat.inspect()
end

function M.open()
	return M.dispatch("open")
end

function M.health()
	return M.dispatch("health")
end

function M.dispatch(action, opts)
	if not coordinator.is_action(action) then
		error("unknown Gator action: " .. tostring(action))
	end
	if not M._coordinator then
		if action == "capture_selection" then
			error("Gator must be set up before capturing context", 0)
		end
		M.setup()
	end
	return M._coordinator:dispatch(action, opts)
end

function M._test()
	assert(M._state, "gator setup must initialize state")
	assert(M._state.config.context.mode == "manual", "manual context must be the default")
	assert(M._state.compatibility.supported, "gator setup must enforce compatible Neovim")
	assert(coordinator.is(M._coordinator), "gator setup must use the production coordinator")
	for name in pairs(M.modules) do
		local module = M.module(name)
		assert(type(module) == "table", name .. " module must return a table")
		assert(module.name == name, name .. " module must identify itself")
		assert(module.api_version == 1, name .. " module must declare API version 1")
	end
	local ok = pcall(M.module, "missing")
	assert(not ok, "unknown modules must fail explicitly")
end

return M
