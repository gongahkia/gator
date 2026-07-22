local M = {}
local redact = require("gator.policy.redact")

M.schema_version = 2
M.source_precedence = { defaults = 1, file = 2, setup = 3 }

M.defaults = {
	schema_version = M.schema_version,
	ui = {
		layout = "adaptive",
		keymaps = {},
		screen_reader = true,
		motion = { enabled = true, interval_ms = 120, reduced = false },
	},
	context = {
		mode = "manual",
		trust = "provenance",
		handoff = { author = "user", max_chars = 4096, review = "required" },
	},
	sessions = { transfer = "manual" },
	workspaces = { mode = "project", max_write_runs = 1 },
	persistence = { sharing = "local" },
	telemetry = { enabled = false, redaction_patterns = {} },
}

local function fail(message)
	error("gator configuration: " .. redact.text(tostring(message)), 3)
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

local root_fields = {
	schema_version = true,
	ui = true,
	context = true,
	sessions = true,
	workspaces = true,
	persistence = true,
	telemetry = true,
}

local function schema_version(value)
	if type(value) ~= "number" or value % 1 ~= 0 then
		fail("settings.schema_version must be an integer")
	end
	if value ~= M.schema_version then
		fail("settings.schema_version is unsupported: " .. value)
	end
	return value
end

local function fragment(value)
	if type(value) ~= "table" then
		fail("settings must be an object")
	end
	safe(value, "settings")
	fields(value, root_fields, "settings")
	if value.schema_version ~= nil then
		schema_version(value.schema_version)
	end
	return value
end

local function settings(value)
	fields(value, root_fields, "settings")
	schema_version(value.schema_version)
	fields(value.ui, { layout = true, keymaps = true, screen_reader = true, motion = true }, "settings.ui")
	fields(value.context, { mode = true, trust = true, handoff = true }, "settings.context")
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
	if type(value.ui.motion) ~= "table" then
		fail("ui.motion must be an object")
	end
	local motion = require("gator.ui.motion").resolve(value.ui.motion)
	value.ui.motion = motion
	if not vim.tbl_contains({ "manual", "inspect", "automatic" }, value.context.mode) then
		fail("context.mode must be manual, inspect, or automatic")
	end
	if not vim.tbl_contains({ "provenance", "repository", "manual" }, value.context.trust) then
		fail("context.trust must be provenance, repository, or manual")
	end
	fields(value.context.handoff, { author = true, max_chars = true, review = true }, "settings.context.handoff")
	if not vim.tbl_contains({ "user", "source", "gator" }, value.context.handoff.author) then
		fail("context.handoff.author must be user, source, or gator")
	end
	if
		type(value.context.handoff.max_chars) ~= "number"
		or value.context.handoff.max_chars < 1
		or value.context.handoff.max_chars % 1 ~= 0
	then
		fail("context.handoff.max_chars must be a positive integer")
	end
	if value.context.handoff.review ~= "required" and value.context.handoff.review ~= "optional" then
		fail("context.handoff.review must be required or optional")
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

function M.migrate(value)
	if type(value) ~= "table" then
		fail("legacy settings must be an object")
	end
	local document = vim.deepcopy(value)
	safe(document, "settings")
	fields(document, root_fields, "settings")
	local from_version = document.schema_version or 1
	if type(from_version) ~= "number" or from_version % 1 ~= 0 then
		fail("settings.schema_version must be an integer")
	end
	if from_version == M.schema_version then
		return document, { migrated = false, from_version = from_version, to_version = from_version }
	end
	if from_version ~= 1 then
		fail("settings.schema_version is unsupported: " .. from_version)
	end
	document.schema_version = M.schema_version
	return document, { migrated = true, from_version = from_version, to_version = M.schema_version }
end

local function source(value, index)
	if type(value) ~= "table" then
		fail("configuration source " .. index .. " must be an object")
	end
	fields(value, { source = true, ref = true, settings = true }, "configuration source " .. index)
	if value.source ~= "file" and value.source ~= "setup" then
		fail("configuration source " .. index .. " is unavailable: " .. tostring(value.source))
	end
	if type(value.ref) ~= "string" or value.ref == "" then
		fail("configuration source " .. index .. " ref must be a non-empty string")
	end
	local settings_value, migration = value.settings
	if value.source == "file" then
		settings_value, migration = M.migrate(settings_value)
	end
	return {
		source = value.source,
		ref = value.ref,
		settings = fragment(settings_value),
		migration = migration,
		index = index,
	}
end

local function record_provenance(result, value, source_value, ref, path)
	if type(value) == "table" and not vim.islist(value) then
		if next(value) == nil and path ~= "" then
			result[path] = { source = source_value, ref = ref }
			return
		end
		for key, child in pairs(value) do
			record_provenance(result, child, source_value, ref, path == "" and key or path .. "." .. key)
		end
		return
	end
	if path ~= "" then
		result[path] = { source = source_value, ref = ref }
	end
end

function M.resolve_sources(values)
	if values == nil then
		values = {}
	end
	if type(values) ~= "table" or not vim.islist(values) then
		fail("configuration sources must be an array")
	end
	local sources, seen = {}, {}
	for index, value in ipairs(values) do
		local entry = source(value, index)
		if seen[entry.source] then
			fail("configuration source is duplicated: " .. entry.source)
		end
		seen[entry.source] = true
		table.insert(sources, entry)
	end
	table.sort(sources, function(left, right)
		local left_priority = M.source_precedence[left.source]
		local right_priority = M.source_precedence[right.source]
		return left_priority == right_priority and left.index < right.index or left_priority < right_priority
	end)
	local value = vim.deepcopy(M.defaults)
	local provenance = {}
	local migrations = {}
	record_provenance(provenance, value, "defaults", "gator.defaults", "")
	for _, entry in ipairs(sources) do
		value = vim.tbl_deep_extend("force", value, entry.settings)
		record_provenance(provenance, entry.settings, entry.source, entry.ref, "")
		if entry.migration and entry.migration.migrated then
			provenance.schema_version = {
				source = "migration",
				ref = entry.ref,
				from_version = entry.migration.from_version,
				to_version = entry.migration.to_version,
			}
			table.insert(migrations, {
				source = entry.source,
				ref = entry.ref,
				from_version = entry.migration.from_version,
				to_version = entry.migration.to_version,
			})
		end
	end
	return { settings = settings(value), provenance = vim.deepcopy(provenance), migrations = vim.deepcopy(migrations) }
end

function M.resolve(opts)
	if opts == nil then
		opts = {}
	end
	local value = M.resolve_sources({ { source = "setup", ref = "gator.setup", settings = opts } })
	return value.settings, value.provenance, value.migrations
end

function M.load(path)
	path = path or vim.fn.stdpath("config") .. "/gator.json"
	if type(path) ~= "string" or path == "" then
		fail("settings path must be a non-empty string")
	end
	if vim.fn.filereadable(path) == 0 then
		local value = M.resolve_sources()
		return value.settings, value.provenance, value.migrations
	end
	local ok, document = pcall(vim.json.decode, table.concat(vim.fn.readfile(path), "\n"))
	if not ok or type(document) ~= "table" then
		fail("settings file is not a JSON object: " .. path)
	end
	local value = M.resolve_sources({ { source = "file", ref = path, settings = document } })
	return value.settings, value.provenance, value.migrations
end

return M
