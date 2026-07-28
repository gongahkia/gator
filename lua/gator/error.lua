local M = {
	taxonomy = {
		runtime = {
			unavailable = {
				message = "Runtime capability is unavailable",
				remedy = "Verify the provider capability and local runtime, then retry.",
			},
			launch_failed = {
				message = "Provider runtime failed to launch",
				remedy = "Inspect the provider diagnostic and retry the run.",
			},
			timed_out = {
				message = "Provider runtime timed out",
				remedy = "Retry with a bounded timeout or inspect the provider session.",
			},
			cancelled = {
				message = "Provider runtime was cancelled",
				remedy = "Resume the provider-native session when it is available.",
			},
			protocol_failed = {
				message = "Provider runtime protocol failed",
				remedy = "Update the provider CLI and retry with an advertised transport.",
			},
		},
		recovery = {
			probe_failed = {
				message = "Run recovery probe failed",
				remedy = "Verify the provider runtime and retry recovery.",
			},
			interrupted = {
				message = "Provider run was interrupted",
				remedy = "Inspect the provider-native session before resuming.",
			},
			session_missing = {
				message = "Provider session is unavailable",
				remedy = "Start a new provider-native session only after reviewing the orphaned run.",
			},
			resume_unavailable = {
				message = "Provider session cannot resume",
				remedy = "Preserve the orphaned run and choose an explicit recovery path.",
			},
			reconnect_failed = {
				message = "Provider reconnect failed",
				remedy = "Retry recovery after verifying provider-native authentication and session state.",
			},
		},
	},
}
local Error = {}
local redact = require("gator.policy.redact")
local notice = require("gator.ui.notice")

Error.__index = Error

function Error:__tostring()
	return M.format(self)
end

local function fail(message)
	error("invalid Gator error: " .. message, 3)
end

local function typed(scope, kind, opts)
	if type(scope) ~= "string" or type(kind) ~= "string" or not M.taxonomy[scope] or not M.taxonomy[scope][kind] then
		fail("error taxonomy entry is unknown")
	end
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("taxonomy options must be a table")
	end
	for key in pairs(opts) do
		if key ~= "detail" and key ~= "level" then
			fail("taxonomy options contain unsupported field: " .. tostring(key))
		end
	end
	local definition = M.taxonomy[scope][kind]
	return M.new(scope .. "." .. kind, definition.message, {
		detail = opts.detail,
		level = opts.level,
		remedy = definition.remedy,
	})
end

function M.new(code, message, opts)
	if type(code) ~= "string" or not code:match("^[a-z][a-z0-9_.-]*$") then
		fail("code must be a lowercase dotted identifier")
	end
	if type(message) ~= "string" or message == "" then
		fail("message must be a non-empty string")
	end
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("options must be a table")
	end
	if type(opts.remedy) ~= "string" or opts.remedy == "" then
		fail("remedy must be a non-empty string")
	end
	if opts.detail ~= nil and (type(opts.detail) ~= "string" or opts.detail == "") then
		fail("detail must be a non-empty string when provided")
	end
	if opts.level ~= nil and type(opts.level) ~= "number" then
		fail("level must be a number when provided")
	end

	return setmetatable({
		code = code,
		message = message,
		remedy = opts.remedy,
		detail = opts.detail,
		level = opts.level or vim.log.levels.ERROR,
	}, Error)
end

function M.is(value)
	return getmetatable(value) == Error
end

function M.runtime(kind, opts)
	return typed("runtime", kind, opts)
end

function M.recovery(kind, opts)
	return typed("recovery", kind, opts)
end

function M.classify(value)
	if not M.is(value) then
		fail("value must be created by gator.error.new")
	end
	local scope, kind = value.code:match("^([a-z][a-z0-9_-]*)%.([a-z][a-z0-9_-]*)$")
	if not scope or not M.taxonomy[scope] or not M.taxonomy[scope][kind] then
		return nil
	end
	return { scope = scope, kind = kind }
end

function M.format(value)
	if not M.is(value) then
		fail("value must be created by gator.error.new")
	end
	local lines = { "[" .. value.code .. "] " .. value.message }

	if value.detail then
		table.insert(lines, "Details: " .. value.detail)
	end
	table.insert(lines, "Recovery: " .. value.remedy)
	return redact.text(table.concat(lines, "\n"))
end

function M.notify(value)
	if not M.is(value) then
		fail("value must be created by gator.error.new")
	end
	notice.show(M.format(value), value.level, { title = "Gator" })
	return value
end

function M.raise(value)
	if not M.is(value) then
		fail("value must be created by gator.error.new")
	end
	error(M.format(value), 2)
end

return M
