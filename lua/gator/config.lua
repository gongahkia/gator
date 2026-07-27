local M = {}
local redact = require("gator.policy.redact")

M.schema_version = 9
M.source_precedence = { defaults = 1, file = 2, setup = 3 }

M.defaults = {
	schema_version = M.schema_version,
	ui = {
		layout = "adaptive",
		keymaps = {},
		screen_reader = true,
		icons = "unicode",
		motion = { enabled = true, interval_ms = 120, reduced = false },
		loading = { enabled = true, spinner = "rattles.braille.dots", interval_ms = 0 },
	},
	context = {
		mode = "manual",
		trust = "provenance",
		handoff = {
			author = "user",
			max_chars = 4096,
			max_files = 24,
			max_file_chars = 65536,
			review = "required",
			profile = "full",
			source_summary = false,
		},
	},
	launch = { default_provider = "ask", transport = "auto" },
	permissions = { codex = { sandbox = "workspace_write" } },
	sessions = { transfer = "manual" },
	providers = {
		pi = { user_confirmed = false },
		aider = { user_confirmed = false },
		amp = { user_confirmed = false },
		cline = { user_confirmed = false },
		copilot = { user_confirmed = false },
		cursor = { user_confirmed = false },
		gemini = { user_confirmed = false },
		goose = { user_confirmed = false },
		kimi = { user_confirmed = false },
		vibe = { user_confirmed = false },
	},
	workspaces = { mode = "project" },
	retention = { max_age_days = 30, cleanup_on_start = true, worktrees = "inactive_clean" },
	persistence = { sharing = "local" },
	telemetry = { enabled = false, redaction_patterns = {} },
	budget = { max_tokens = 0, action = "warn", max_concurrent_runs = 0 },
	review = { commands = {} },
	acp = { commands = {} },
	runbooks = { max_concurrent = 0, max_tokens = 0 },
}

local function fail(message)
	error("gator configuration: " .. redact.text(tostring(message)), 3)
end

local function sensitive(key)
	key = key:lower()
	if key == "max_tokens" then
		return false
	end
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
	launch = true,
	permissions = true,
	sessions = true,
	providers = true,
	workspaces = true,
	retention = true,
	persistence = true,
	telemetry = true,
	budget = true,
	review = true,
	acp = true,
	runbooks = true,
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

local provider_names = {
	pi = true,
	aider = true,
	amp = true,
	cline = true,
	copilot = true,
	cursor = true,
	gemini = true,
	goose = true,
	kimi = true,
	vibe = true,
}

local launch_provider_names = vim.tbl_extend("force", { claude = true, codex = true, opencode = true }, provider_names)

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function command_entries(value, path)
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail(path .. " must be an object")
	end
	for name, command in pairs(value) do
		identifier(name, path .. " key")
		fields(command, { argv = true }, path .. "." .. name)
		if type(command.argv) ~= "table" or not vim.islist(command.argv) or #command.argv == 0 then
			fail(path .. "." .. name .. ".argv must be a non-empty array")
		end
		for index, item in ipairs(command.argv) do
			if type(item) ~= "string" or item == "" then
				fail(path .. "." .. name .. ".argv[" .. index .. "] must be non-empty text")
			end
		end
	end
	return value
end

local function settings(value)
	fields(value, root_fields, "settings")
	schema_version(value.schema_version)
	fields(
		value.ui,
		{ layout = true, keymaps = true, screen_reader = true, icons = true, motion = true, loading = true },
		"settings.ui"
	)
	fields(value.context, { mode = true, trust = true, handoff = true }, "settings.context")
	fields(value.launch, { default_provider = true, transport = true }, "settings.launch")
	fields(value.permissions, { codex = true }, "settings.permissions")
	fields(value.permissions.codex, { sandbox = true }, "settings.permissions.codex")
	fields(value.sessions, { transfer = true }, "settings.sessions")
	fields(value.providers, provider_names, "settings.providers")
	for name in pairs(provider_names) do
		fields(value.providers[name], { user_confirmed = true }, "settings.providers." .. name)
	end
	fields(value.workspaces, { mode = true }, "settings.workspaces")
	fields(value.retention, { max_age_days = true, cleanup_on_start = true, worktrees = true }, "settings.retention")
	fields(value.persistence, { sharing = true }, "settings.persistence")
	fields(value.telemetry, { enabled = true, redaction_patterns = true }, "settings.telemetry")
	fields(value.budget, { max_tokens = true, action = true, max_concurrent_runs = true }, "settings.budget")
	fields(value.review, { commands = true }, "settings.review")
	fields(value.acp, { commands = true }, "settings.acp")
	fields(value.runbooks, { max_concurrent = true, max_tokens = true }, "settings.runbooks")
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
	if not vim.tbl_contains({ "unicode", "nerd_font", "ascii", "none" }, value.ui.icons) then
		fail("ui.icons must be unicode, nerd_font, ascii, or none")
	end
	if type(value.ui.motion) ~= "table" then
		fail("ui.motion must be an object")
	end
	local motion = require("gator.ui.motion").resolve(value.ui.motion)
	value.ui.motion = motion
	value.ui.loading = require("gator.ui.loading").resolve(value.ui.loading)
	if not vim.tbl_contains({ "manual", "inspect", "automatic" }, value.context.mode) then
		fail("context.mode must be manual, inspect, or automatic")
	end
	if not vim.tbl_contains({ "provenance", "repository", "manual" }, value.context.trust) then
		fail("context.trust must be provenance, repository, or manual")
	end
	fields(value.context.handoff, {
		author = true,
		max_chars = true,
		max_files = true,
		max_file_chars = true,
		review = true,
		profile = true,
		source_summary = true,
	}, "settings.context.handoff")
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
	for _, field in ipairs({ "max_files", "max_file_chars" }) do
		if
			type(value.context.handoff[field]) ~= "number"
			or value.context.handoff[field] < 0
			or value.context.handoff[field] % 1 ~= 0
		then
			fail("context.handoff." .. field .. " must be a non-negative integer")
		end
	end
	if value.context.handoff.review ~= "required" and value.context.handoff.review ~= "optional" then
		fail("context.handoff.review must be required or optional")
	end
	if not vim.tbl_contains({ "full", "compact", "summary-first" }, value.context.handoff.profile) then
		fail("context.handoff.profile must be full, compact, or summary-first")
	end
	if type(value.context.handoff.source_summary) ~= "boolean" then
		fail("context.handoff.source_summary must be boolean")
	end
	command_entries(value.review.commands, "settings.review.commands")
	command_entries(value.acp.commands, "settings.acp.commands")
	local allowed_launch_providers = vim.deepcopy(launch_provider_names)
	for name in pairs(value.acp.commands) do
		allowed_launch_providers[name] = true
	end
	if value.launch.default_provider ~= "ask" and not allowed_launch_providers[value.launch.default_provider] then
		fail("launch.default_provider must be ask or a supported provider")
	end
	if not vim.tbl_contains({ "auto", "chat", "terminal" }, value.launch.transport) then
		fail("launch.transport must be auto, chat, or terminal")
	end
	if not vim.tbl_contains({ "workspace_write", "read_only" }, value.permissions.codex.sandbox) then
		fail("permissions.codex.sandbox must be workspace_write or read_only")
	end
	if value.sessions.transfer ~= "manual" then
		fail("sessions.transfer must be manual")
	end
	for name in pairs(provider_names) do
		if type(value.providers[name].user_confirmed) ~= "boolean" then
			fail("providers." .. name .. ".user_confirmed must be boolean")
		end
	end
	if not vim.tbl_contains({ "project", "worktree" }, value.workspaces.mode) then
		fail("workspaces.mode must be project or worktree")
	end
	if
		type(value.retention.max_age_days) ~= "number"
		or value.retention.max_age_days < 0
		or value.retention.max_age_days % 1 ~= 0
	then
		fail("retention.max_age_days must be a non-negative integer")
	end
	if type(value.retention.cleanup_on_start) ~= "boolean" then
		fail("retention.cleanup_on_start must be boolean")
	end
	if value.retention.worktrees ~= "inactive_clean" then
		fail("retention.worktrees must be inactive_clean")
	end
	if value.persistence.sharing ~= "local" then
		fail("persistence.sharing must be local")
	end
	if type(value.telemetry.enabled) ~= "boolean" then
		fail("telemetry.enabled must be boolean")
	end
	if type(value.budget.max_tokens) ~= "number" or value.budget.max_tokens < 0 or value.budget.max_tokens % 1 ~= 0 then
		fail("budget.max_tokens must be a non-negative integer")
	end
	if value.budget.action ~= "warn" and value.budget.action ~= "stop" then
		fail("budget.action must be warn or stop")
	end
	if
		type(value.budget.max_concurrent_runs) ~= "number"
		or value.budget.max_concurrent_runs < 0
		or value.budget.max_concurrent_runs % 1 ~= 0
	then
		fail("budget.max_concurrent_runs must be a non-negative integer")
	end
	for _, field in ipairs({ "max_concurrent", "max_tokens" }) do
		if type(value.runbooks[field]) ~= "number" or value.runbooks[field] < 0 or value.runbooks[field] % 1 ~= 0 then
			fail("runbooks." .. field .. " must be a non-negative integer")
		end
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
	if
		from_version ~= 1
		and from_version ~= 2
		and from_version ~= 3
		and from_version ~= 4
		and from_version ~= 5
		and from_version ~= 6
		and from_version ~= 7
		and from_version ~= 8
	then
		fail("settings.schema_version is unsupported: " .. from_version)
	end
	document.launch = document.launch or {}
	document.permissions = document.permissions or {}
	document.permissions.codex = document.permissions.codex or {}
	document.permissions.codex.sandbox = document.permissions.codex.sandbox or "workspace_write"
	document.workspaces = document.workspaces or {}
	document.workspaces.max_write_runs = nil
	document.retention = document.retention or {}
	if document.retention.max_age_days == nil then
		document.retention.max_age_days = 30
	end
	if document.retention.cleanup_on_start == nil then
		document.retention.cleanup_on_start = true
	end
	if document.retention.worktrees == nil then
		document.retention.worktrees = "inactive_clean"
	end
	document.context = document.context or {}
	document.context.handoff = document.context.handoff or {}
	document.context.handoff.profile = document.context.handoff.profile or "full"
	document.context.handoff.max_files = document.context.handoff.max_files or 24
	document.context.handoff.max_file_chars = document.context.handoff.max_file_chars or 65536
	if document.context.handoff.source_summary == nil then
		document.context.handoff.source_summary = false
	end
	document.ui = document.ui or {}
	document.ui.loading = document.ui.loading or {}
	document.budget = document.budget or {}
	document.budget.max_concurrent_runs = document.budget.max_concurrent_runs or 0
	document.review = document.review or {}
	document.acp = document.acp or {}
	document.runbooks = document.runbooks or {}
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
	if
		type(settings_value) == "table"
		and settings_value.schema_version ~= nil
		and settings_value.schema_version ~= M.schema_version
	then
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
