local M = {}
local Error = {}
local redact = require("gator.policy.redact")

Error.__index = Error

function Error:__tostring()
	return M.format(self)
end

local function fail(message)
	error("invalid Gator error: " .. message, 3)
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
	vim.notify(M.format(value), value.level, { title = "Gator" })
	return value
end

function M.raise(value)
	if not M.is(value) then
		fail("value must be created by gator.error.new")
	end
	error(M.format(value), 2)
end

return M
