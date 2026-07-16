local test = assert(vim.g.gator_test, "Gator test environment is not initialized")
local M = {}
local tempdirs = 0

function M.tempdir(name)
	assert(type(name) == "string" and name:match("^[%w_-]+$"), "test tempdir name is invalid")
	tempdirs = tempdirs + 1
	local path = test.tempdir .. "/" .. name .. "-" .. tempdirs
	assert(vim.fn.mkdir(path, "p") == 1, "must create test tempdir: " .. path)
	return path
end

function M.fixture_path(name)
	assert(type(name) == "string" and not name:find("..", 1, true), "fixture name must stay within tests/fixtures")
	local path = test.root .. "/tests/fixtures/" .. name
	assert(vim.fn.filereadable(path) == 1, "fixture is missing: " .. name)
	return path
end

function M.read(path)
	local file, err = io.open(path, "rb")
	assert(file, err)
	local content = file:read("*a")
	file:close()
	return content
end

function M.write(path, content)
	assert(vim.fn.mkdir(vim.fn.fnamemodify(path, ":h"), "p") >= 0, "must create fixture parent")
	local file, err = io.open(path, "wb")
	assert(file, err)
	file:write(content)
	file:close()
end

function M.cleanup()
	vim.fn.delete(test.tempdir, "rf")
end

return M
