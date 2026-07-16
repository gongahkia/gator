local M = {}
local redact = require("gator.policy.redact")

M.defaults = {
	ui = { layout = "adaptive", keymaps = {}, screen_reader = true },
	context = { mode = "manual", trust = "provenance" },
	sessions = { transfer = "manual" },
	workspaces = { mode = "project", max_write_runs = 1 },
	persistence = { sharing = "local" },
	telemetry = { enabled = false, redaction_patterns = {} },
}

local function fail(message)
	error("gator configuration: " .. message, 3)
end

local function sensitive(key)
	key = key:lower()
	return key:match("token")
		or key:match("secret")
		or key:match("credential")
		or key:match("password")
		or key:match("api[_-]?key")
		or key:match("access[_-]?key")
end

local function safe(value, path)
	local kind = type(value)
	if kind == "string" or kind == "number" or kind == "boolean" then
		return
	end
	if kind ~= "table" then
		fail(path .. " must be JSON-compatible")
	end
	if vim.islist(value) then
		for index, child in ipairs(value) do
			safe(child, path .. "[" .. index .. "]")
		end
		return
	end
	for key, child in pairs(value) do
		if type(key) ~= "string" then
			fail(path .. " keys must be strings")
		end
		if sensitive(key) then
			fail(path .. " must not store provider credentials")
		end
		safe(child, path .. "." .. key)
	end
end

local function fields(value, allowed, path)
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail(path .. " must be an object")
	end
	for key in pairs(value) do
		if not allowed[key] then
			fail(path .. " contains unsupported field: " .. tostring(key))
		end
	end
end

local function settings(value)
	fields(
		value,
		{ ui = true, context = true, sessions = true, workspaces = true, persistence = true, telemetry = true },
		"settings"
	)
	fields(value.ui, { layout = true, keymaps = true, screen_reader = true }, "settings.ui")
	fields(value.context, { mode = true, trust = true }, "settings.context")
	fields(value.sessions, { transfer = true }, "settings.sessions")
	fields(value.workspaces, { mode = true, max_write_runs = true }, "settings.workspaces")
	fields(value.persistence, { sharing = true }, "settings.persistence")
	fields(value.telemetry, { enabled = true, redaction_patterns = true }, "settings.telemetry")
	if not vim.tbl_contains({ "adaptive", "modal" }, value.ui.layout) then
		fail("ui.layout must be adaptive or modal")
	end
	if type(value.ui.keymaps) ~= "table" then
		fail("ui.keymaps must be an object")
	end
	for name, mapping in pairs(value.ui.keymaps) do
		if type(name) ~= "string" or name == "" or type(mapping) ~= "string" or mapping == "" then
			fail("ui.keymaps must map non-empty names to non-empty mappings")
		end
	end
	if type(value.ui.screen_reader) ~= "boolean" then
		fail("ui.screen_reader must be boolean")
	end
	if not vim.tbl_contains({ "manual", "inspect", "automatic" }, value.context.mode) then
		fail("context.mode must be manual, inspect, or automatic")
	end
	if not vim.tbl_contains({ "provenance", "repository", "manual" }, value.context.trust) then
		fail("context.trust must be provenance, repository, or manual")
	end
	if value.sessions.transfer ~= "manual" then
		fail("sessions.transfer must be manual")
	end
	if not vim.tbl_contains({ "project", "worktree" }, value.workspaces.mode) then
		fail("workspaces.mode must be project or worktree")
	end
	if
		type(value.workspaces.max_write_runs) ~= "number"
		or value.workspaces.max_write_runs < 1
		or value.workspaces.max_write_runs % 1 ~= 0
	then
		fail("workspaces.max_write_runs must be a positive integer")
	end
	if value.persistence.sharing ~= "local" then
		fail("persistence.sharing must be local")
	end
	if type(value.telemetry.enabled) ~= "boolean" then
		fail("telemetry.enabled must be boolean")
	end
	redact.validate_patterns(value.telemetry.redaction_patterns)
	return value
end

function M.resolve(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("settings must be an object")
	end
	safe(opts, "settings")
	fields(
		opts,
		{ ui = true, context = true, sessions = true, workspaces = true, persistence = true, telemetry = true },
		"settings"
	)
	local config = vim.tbl_deep_extend("force", vim.deepcopy(M.defaults), opts)
	return settings(config)
end

function M.load(path)
	path = path or vim.fn.stdpath("config") .. "/gator.json"
	if type(path) ~= "string" or path == "" then
		fail("settings path must be a non-empty string")
	end
	if vim.fn.filereadable(path) == 0 then
		return M.resolve({})
	end
	local ok, document = pcall(vim.json.decode, table.concat(vim.fn.readfile(path), "\n"))
	if not ok or type(document) ~= "table" then
		fail("settings file is not a JSON object: " .. path)
	end
	return M.resolve(document)
end

return M
