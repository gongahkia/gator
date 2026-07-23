local filesystem = require("gator.core.filesystem")
local redact = require("gator.policy.redact")
local state = require("gator.state")

local M = {
	api_version = 1,
	schema_version = 1,
	states = { ready = true, unavailable = true, failed = true, cancelled = true },
}

local function fail(message)
	error("Gator diagnostic export: " .. redact.text(tostring(message)), 3)
end

local function timestamp(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer timestamp")
	end
	return value
end

local function root(value)
	if type(value) ~= "string" or value == "" or value:sub(1, 1) ~= "/" then
		fail("root must be an absolute path")
	end
	return vim.fs.normalize(value)
end

local function policy(value)
	if type(value) ~= "table" then
		fail("storage must be an object")
	end
	for key in pairs(value) do
		if key ~= "sharing" then
			fail("storage contains unsupported field: " .. tostring(key))
		end
	end
	if value.sharing ~= "local" then
		return nil, "diagnostic export is unavailable because local-only storage is not configured"
	end
	return { sharing = "local" }
end

local function compatibility(value)
	if type(value) ~= "table" or type(value.supported) ~= "boolean" then
		fail("state compatibility is invalid")
	end
	local result = { supported = value.supported }
	if type(value.version) == "table" then
		local version = {}
		for _, key in ipairs({ "major", "minor", "patch" }) do
			if type(value.version[key]) ~= "number" or value.version[key] < 0 or value.version[key] % 1 ~= 0 then
				fail("state compatibility version is invalid")
			end
			version[key] = value.version[key]
		end
		result.neovim = version
	end
	return result
end

local function workspace(value)
	if type(value) ~= "table" or type(value.status) ~= "string" then
		fail("state workspace is invalid")
	end
	return { status = value.status }
end

local function configuration(value)
	if type(value) ~= "table" then
		fail("state configuration is invalid")
	end
	return {
		schema_version = timestamp(value.schema_version, "state configuration schema_version"),
		context = { mode = value.context.mode, trust = value.context.trust },
		sessions = { transfer = value.sessions.transfer },
		workspaces = { mode = value.workspaces.mode, max_write_runs = value.workspaces.max_write_runs },
		persistence = { sharing = value.persistence.sharing },
		telemetry = { enabled = value.telemetry.enabled },
	}
end

local function report(value, captured_at)
	if not state.is(value) then
		fail("capture requires a Gator state store")
	end
	local snapshot = value:snapshot()
	return {
		schema_version = M.schema_version,
		captured_at = timestamp(captured_at, "captured_at"),
		product = "gator",
		network_telemetry = false,
		retention = { mode = "rolling", files = 1 },
		compatibility = compatibility(snapshot.compatibility),
		workspace = workspace(snapshot.workspace),
		configuration = configuration(snapshot.config),
	}
end

local function cancellation(callback)
	if callback == nil then
		return false
	end
	local ok, value = pcall(callback)
	if not ok or type(value) ~= "boolean" then
		return nil, redact.text(ok and "cancel callback must return a boolean" or tostring(value))
	end
	return value
end

function M.capture(opts)
	if type(opts) ~= "table" then
		fail("capture requires options")
	end
	for key in pairs(opts) do
		if key ~= "state" and key ~= "captured_at" then
			fail("capture contains unsupported field: " .. tostring(key))
		end
	end
	return report(opts.state, opts.captured_at or os.time())
end

function M.write(opts)
	if type(opts) ~= "table" then
		fail("write requires options")
	end
	for key in pairs(opts) do
		if
			key ~= "state"
			and key ~= "root"
			and key ~= "storage"
			and key ~= "captured_at"
			and key ~= "cancel"
			and key ~= "filesystem"
		then
			fail("write contains unsupported field: " .. tostring(key))
		end
	end
	if opts.cancel ~= nil and type(opts.cancel) ~= "function" then
		fail("cancel must be a function")
	end
	if opts.filesystem ~= nil and not filesystem.is(opts.filesystem) then
		fail("filesystem must be a Gator filesystem boundary")
	end
	local storage, unavailable = policy(opts.storage)
	local base = root(opts.root or (vim.fn.stdpath("state") .. "/gator"))
	local directory = base .. "/diagnostics"
	local path = directory .. "/current.json"
	if not storage then
		return { state = "unavailable", path = path, reason = unavailable }
	end
	local cancelled, cancellation_reason = cancellation(opts.cancel)
	if cancelled == nil then
		return { state = "failed", path = path, reason = cancellation_reason }
	end
	if cancelled then
		return { state = "cancelled", path = path, reason = "diagnostic export was cancelled" }
	end
	local value = report(opts.state, opts.captured_at or os.time())
	local boundary = opts.filesystem or filesystem.new()
	local temporary = path .. ".tmp-" .. vim.uv.hrtime()
	local ok, reason = pcall(function()
		if not boundary:mkdir(directory) then
			fail("cannot create diagnostic directory")
		end
		if not boundary:write(temporary, vim.json.encode(value)) then
			fail("cannot write diagnostic export")
		end
		if not boundary:rename(temporary, path) then
			boundary:remove(temporary)
			fail("cannot replace diagnostic export")
		end
	end)
	if not ok then
		pcall(boundary.remove, boundary, temporary)
		return { state = "failed", path = path, reason = redact.text(tostring(reason)) }
	end
	return { state = "ready", path = path, report = vim.deepcopy(value) }
end

return M
