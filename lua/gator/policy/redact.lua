local M = {}
local Redactor = {}

Redactor.__index = Redactor

local placeholder = "[REDACTED]"
local known = {
	{ pattern = "([Aa]uthorization%s*:%s*[Bb]earer%s+)[^%s,;]+" },
	{ pattern = "([Bb]earer%s+)[%w%-%._~%+/=]+" },
	{ pattern = "([Tt][Oo][Kk][Ee][Nn]%s*[:=]%s*[\"']?)[^%s,;\"'}]+" },
	{ pattern = "([Ss][Ee][Cc][Rr][Ee][Tt]%s*[:=]%s*[\"']?)[^%s,;\"'}]+" },
	{ pattern = "([Pp][Aa][Ss][Ss][Ww][Oo][Rr][Dd]%s*[:=]%s*[\"']?)[^%s,;\"'}]+" },
	{ pattern = "([Cc][Rr][Ee][Dd][Ee][Nn][Tt][Ii][Aa][Ll]%s*[:=]%s*[\"']?)[^%s,;\"'}]+" },
	{ pattern = "([Aa][Pp][Ii][_%-%s]?[Kk][Ee][Yy]%s*[:=]%s*[\"']?)[^%s,;\"'}]+" },
	{ pattern = "gh[pousr]_[%w_%-]+", replace = placeholder },
	{ pattern = "github_pat_[%w_%-]+", replace = placeholder },
	{ pattern = "sk%-[%w_%-]+", replace = placeholder },
}

local function fail(message)
	error("Gator redaction: " .. message, 3)
end

local function sensitive(key)
	key = key:lower()
	return key:match("token")
		or key:match("secret")
		or key:match("credential")
		or key:match("password")
		or key:match("authorization")
		or key:match("api[_-]?key")
		or key:match("access[_-]?key")
end

function M.validate_patterns(value)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("patterns must be a list")
	end
	local result = {}
	for index, pattern in ipairs(value) do
		if type(pattern) ~= "string" or pattern == "" then
			fail("patterns[" .. index .. "] must be a non-empty Lua pattern")
		end
		local ok, first, last = pcall(string.find, "", pattern)
		if not ok then
			fail("patterns[" .. index .. "] is invalid")
		end
		if first and last < first then
			fail("patterns[" .. index .. "] must not match an empty string")
		end
		result[index] = pattern
	end
	return result
end

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "patterns" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local patterns = opts.patterns
	if patterns == nil then
		patterns = {}
	end
	return setmetatable({ patterns = M.validate_patterns(patterns) }, Redactor)
end

function Redactor:text(value)
	if type(value) ~= "string" then
		fail("text must be a string")
	end
	local result = value
	for _, rule in ipairs(known) do
		if rule.replace then
			result = result:gsub(rule.pattern, rule.replace)
		else
			result = result:gsub(rule.pattern, function(prefix)
				return prefix .. placeholder
			end)
		end
	end
	for _, pattern in ipairs(self.patterns) do
		result = result:gsub(pattern, placeholder)
	end
	return result
end

function Redactor:value(value)
	local kind = type(value)
	if kind == "string" then
		return self:text(value)
	end
	if kind == "number" or kind == "boolean" then
		return value
	end
	if kind ~= "table" then
		fail("value must be JSON-compatible")
	end
	local result = {}
	if vim.islist(value) then
		for index, item in ipairs(value) do
			result[index] = self:value(item)
		end
		return result
	end
	for key, item in pairs(value) do
		if type(key) ~= "string" then
			fail("value keys must be strings")
		end
		result[key] = sensitive(key) and placeholder or self:value(item)
	end
	return result
end

local default = M.new({ patterns = {} })

function M.configure(opts)
	default = M.new(opts)
	return default
end

function M.text(value)
	return default:text(value)
end

function M.prompt(value)
	return default:text(value)
end

function M.value(value)
	return default:value(value)
end

function M.log(opts)
	if type(opts) ~= "table" then
		fail("log requires options")
	end
	for key in pairs(opts) do
		if key ~= "message" and key ~= "fields" and key ~= "write" then
			fail("log contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.write) ~= "function" then
		fail("log requires a write callback")
	end
	local record = {
		message = default:text(opts.message),
		fields = default:value(opts.fields or {}),
	}
	opts.write(vim.deepcopy(record))
	return record
end

return M
