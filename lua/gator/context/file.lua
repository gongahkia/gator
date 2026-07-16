local pack = require("gator.context.pack")
local M = {}

local function fail(message)
	error("Gator file context: " .. message, 3)
end

local function buffer(value)
	if type(value) ~= "number" or value < 1 or value % 1 ~= 0 or not vim.api.nvim_buf_is_valid(value) then
		fail("buffer must be valid")
	end
	local name = vim.api.nvim_buf_get_name(value)
	if name == "" then
		fail("buffer must name an existing file")
	end
	local path = vim.uv.fs_realpath(name)
	if not path or vim.fn.filereadable(path) ~= 1 then
		fail("buffer must name an existing file")
	end
	return value, path
end

local function content(value)
	local lines = vim.api.nvim_buf_get_lines(value, 0, -1, false)
	local text = table.concat(lines, "\n")
	if #lines > 0 and vim.bo[value].endofline then
		text = text .. "\n"
	end
	return text
end

function M.capture(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("options must be a table")
	end
	for key in pairs(opts) do
		if key ~= "buffer" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local source, path = buffer(opts.buffer or vim.api.nvim_get_current_buf())
	local digest = vim.fn.sha256(content(source))
	local reference = "file://" .. path .. "#sha256=" .. digest
	local entry = pack.entry({
		id = "file-" .. digest,
		kind = "file",
		ref = reference,
		provenance = { source = "buffer", ref = path },
		trust = "manual",
		token_estimate = { status = "unavailable", reason = "file content has not been provider-counted" },
		transfer = { eligible = true },
	})
	return {
		entry = entry,
		path = path,
		digest = digest,
		revision = vim.api.nvim_buf_get_changedtick(source),
		language = vim.bo[source].filetype,
	}
end

return M
