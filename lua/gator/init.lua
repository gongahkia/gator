local compat = require("gator.compat")
local coordinator = require("gator.coordinator")

local M = {
	_state = nil,
	_coordinator = nil,
	error = require("gator.error"),
	modules = {
		core = "gator.core",
		coordinator = "gator.coordinator",
		ui = "gator.ui",
		adapters = "gator.adapters",
		context = "gator.context",
		extensions = "gator.extensions",
		github = "gator.github",
		indexer = "gator.indexer",
		workspace = "gator.workspace",
		review = "gator.review",
		policy = "gator.policy",
		performance = "gator.performance",
		startup = "gator.startup",
		telemetry = "gator.telemetry",
	},
}

function M.module(name)
	local path = M.modules[name]
	if not path then
		error("unknown Gator module: " .. tostring(name))
	end
	return require(path)
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
	if not M._state then
		M.setup()
	end
	return M._coordinator:open()
end

function M.health()
	if not M._state then
		M.setup()
	end
	return M._coordinator:health()
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
