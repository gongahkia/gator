local compat = require("gator.compat")
local coordinator = require("gator.coordinator")

local M = {
	_state = nil,
	_coordinator = nil,
	error = require("gator.error"),
	modules = coordinator.modules,
}
local source = debug.getinfo(1, "S").source
local runtime_root = source:sub(1, 1) == "@" and vim.fs.dirname(vim.fs.dirname(vim.fs.dirname(source:sub(2)))) or nil

local function restore_runtimepath()
	if not runtime_root then
		return
	end
	for _, entry in ipairs(vim.opt.runtimepath:get()) do
		if vim.fs.normalize(entry) == runtime_root then
			return
		end
	end
	vim.opt.runtimepath:append(runtime_root)
end

local function retain_runtimepath()
	restore_runtimepath()
	local group = vim.api.nvim_create_augroup("GatorRuntimePath", { clear = true })
	vim.api.nvim_create_autocmd("VimEnter", { group = group, once = true, callback = restore_runtimepath })
end

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
	retain_runtimepath()
	require("gator.commands").register()
	local next = coordinator.new(opts)
	if M._coordinator then
		M._coordinator:dispose()
	end
	M._coordinator = next
	M._coordinator:load_extensions()
	M._state = M._coordinator:state()
	M._coordinator:bootstrap_recovery()
	return M
end

function M.on(event, handler)
	if not M._coordinator then
		M.setup()
	end
	return M._coordinator:extensions_runtime():subscribe(event, handler)
end

function M.extensions()
	if not M._coordinator then
		M.setup()
	end
	return M._coordinator:extensions_runtime():status()
end

function M.statusline(opts)
	if not M._coordinator then
		M.setup()
	end
	return M._coordinator:statusline(opts)
end

function M.compatibility()
	return compat.inspect()
end

function M.compatibility_manifest(opts)
	return compat.manifest(opts)
end

function M.compatibility_manifest_json(opts)
	return compat.manifest_json(opts)
end

function M.inspect()
	if not M._coordinator then
		M.setup()
	end
	return M._coordinator:inspect()
end

function M.open(opts)
	return M.dispatch("open", opts)
end

function M.launch(opts)
	if type(opts) ~= "table" then
		return M.dispatch("open")
	end
	if not M._coordinator then
		M.setup()
	end
	if type(opts.objective) == "string" and vim.trim(opts.objective) ~= "" then
		return M._coordinator:workflow():choose(opts)
	end
	return M.dispatch("open", opts)
end

function M.runs()
	return M.dispatch("runs")
end

function M.events(run_id)
	return M.dispatch("events", { run_id = run_id })
end

function M.handoff(run_id, provider, opts)
	if not M._coordinator then
		M.setup()
	end
	return M._coordinator:workflow():handoff(run_id, provider, opts)
end

function M.send_context(opts)
	return M.dispatch("send_context", opts)
end

function M.review(run_id)
	return M.dispatch("review", { run_id = run_id })
end

function M.create_runbook(opts)
	if not M._coordinator then
		M.setup()
	end
	return M._coordinator:workflow():create_runbook(opts)
end

function M.runbook_status(id)
	if not M._coordinator then
		M.setup()
	end
	return M._coordinator:workflow():runbook_status(id)
end

function M.runbook_next()
	return M.dispatch("runbook_next")
end

function M.health()
	return M.dispatch("health")
end

function M.export_diagnostics()
	return M.dispatch("export_diagnostics")
end

function M.verify_beta_readiness()
	return M.dispatch("verify_beta_readiness")
end

function M.stop_session()
	return M.dispatch("stop_session")
end

function M.prune()
	return M.dispatch("prune")
end

function M.storage()
	return M.dispatch("storage")
end

function M.forget(run_id)
	return M.dispatch("forget", { run_id = run_id })
end

function M.dispatch(action, opts)
	if not coordinator.is_action(action) then
		error("unknown Gator action: " .. tostring(action))
	end
	if not M._coordinator then
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
		assert(
			type(module.api_version) == "number" and module.api_version >= 1,
			name .. " module must declare a supported API version"
		)
	end
	local ok = pcall(M.module, "missing")
	assert(not ok, "unknown modules must fail explicitly")
end

return M
