local redact = require("gator.policy.redact")

local M = {}

local function fail(message)
	error("Gator image attachments: " .. redact.text(tostring(message)), 3)
end

local function run_id(value)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail("run id must be a lowercase identifier")
	end
	return value
end

local function directory(value)
	return vim.fn.stdpath("state") .. "/gator/attachments/" .. run_id(value)
end

local function apple_string(value)
	return value:gsub("\\", "\\\\"):gsub('"', '\\"')
end

function M.directory(value)
	return directory(value)
end

function M.capture(opts)
	if type(opts) ~= "table" or opts.enabled ~= true then
		fail("image capture is disabled")
	end
	local path = directory(opts.run_id)
	vim.fn.mkdir(path, "p")
	local target = path .. "/clipboard-" .. tostring(vim.uv.hrtime()) .. ".png"
	if vim.fn.has("mac") ~= 1 then
		return nil, "clipboard image capture is currently available on macOS only"
	end
	local script = table.concat({
		"set imageData to the clipboard as «class PNGf»",
		' set imageFile to POSIX file "' .. apple_string(target) .. '"',
		"set imageRef to open for access imageFile with write permission",
		"set eof imageRef to 0",
		"write imageData to imageRef",
		"close access imageRef",
	}, "\n")
	local result = vim.system({ "osascript", "-e", script }, { text = true }):wait()
	if result.code ~= 0 or vim.fn.filereadable(target) ~= 1 then
		pcall(vim.fn.delete, target)
		return nil, "clipboard does not contain a PNG image"
	end
	local stat = vim.uv.fs_stat(target)
	if not stat or stat.size < 1 then
		pcall(vim.fn.delete, target)
		return nil, "clipboard image is empty"
	end
	return { path = target, bytes = stat.size, source = "clipboard" }
end

function M.remove_run(value)
	local path = directory(value)
	if vim.fn.isdirectory(path) == 1 then
		vim.fn.delete(path, "rf")
	end
	return true
end

return M
