local redact = require("gator.policy.redact")

local M = {}

local function fail(message)
	error("Gator context references: " .. redact.text(tostring(message)), 3)
end

local function inside(path, root)
	return path == root or vim.startswith(path, root .. "/")
end

local function root(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	local resolved = vim.uv.fs_realpath(vim.fn.expand(value))
	if not resolved or vim.fn.isdirectory(resolved) ~= 1 then
		fail(name .. " must resolve to a directory")
	end
	return vim.fs.normalize(resolved)
end

local function relative(value, name)
	if type(value) ~= "string" or value == "" or value:sub(1, 1) == "/" or value:find("\\", 1, true) then
		fail(name .. " must be a normalized relative path")
	end
	for part in value:gmatch("[^/]+") do
		if part == "." or part == ".." then
			fail(name .. " must not traverse directories")
		end
	end
	return value
end

local function read(path, maximum)
	local stat = vim.uv.fs_stat(path)
	if not stat or stat.type ~= "file" then
		return nil
	end
	if stat.size > maximum then
		return nil, "exceeds per-file limit"
	end
	local handle = vim.uv.fs_open(path, "r", 420)
	if not handle then
		return nil, "unreadable"
	end
	local value = vim.uv.fs_read(handle, stat.size, 0)
	vim.uv.fs_close(handle)
	if type(value) ~= "string" or value:find("%z") then
		return nil, "not text"
	end
	return value
end

local function project_roots(workspace)
	local path = workspace .. "/.gator/references.json"
	if vim.fn.filereadable(path) ~= 1 then
		return {}
	end
	local ok, document = pcall(vim.json.decode, table.concat(vim.fn.readfile(path), "\n"))
	if not ok or type(document) ~= "table" or vim.islist(document) then
		fail("project references must be a JSON object")
	end
	for key in pairs(document) do
		if key ~= "roots" then
			fail("project references contain unsupported field: " .. tostring(key))
		end
	end
	if type(document.roots) ~= "table" or not vim.islist(document.roots) then
		fail("project references roots must be an array")
	end
	local values = {}
	for index, value in ipairs(document.roots) do
		local candidate = root(
			workspace .. "/" .. relative(value, "project references roots[" .. index .. "]"),
			"project references root"
		)
		if not inside(candidate, workspace) then
			fail("project references roots must remain inside the Git workspace")
		end
		table.insert(values, { path = candidate, source = ".gator/references.json" })
	end
	return values
end

local function roots(workspace, settings)
	local values, seen = {}, {}
	for index, value in ipairs(settings.roots) do
		local candidate = root(value, "context.references.roots[" .. index .. "]")
		if not seen[candidate] then
			seen[candidate] = true
			table.insert(values, { path = candidate, source = "gator.setup" })
		end
	end
	for _, value in ipairs(project_roots(workspace)) do
		if not seen[value.path] then
			seen[value.path] = true
			table.insert(values, value)
		end
	end
	return values
end

local function skill_items(workspace, settings)
	local items = {}
	for _, configured in ipairs(roots(workspace, settings)) do
		local handle = vim.uv.fs_scandir(configured.path)
		while handle do
			local name, kind = vim.uv.fs_scandir_next(handle)
			if not name then
				break
			end
			if kind == "directory" then
				local path = configured.path .. "/" .. name .. "/SKILL.md"
				if vim.fn.filereadable(path) == 1 then
					table.insert(items, { name = name, path = path, source = configured.source })
				end
			end
		end
	end
	table.sort(items, function(left, right)
		return left.name == right.name and left.path < right.path or left.name < right.name
	end)
	return items
end

local function tracked(workspace)
	local result = vim.system({ "git", "ls-files", "--cached", "--", "." }, { cwd = workspace, text = true }):wait()
	if result.code ~= 0 then
		fail("Git tracked-file lookup failed")
	end
	local values = {}
	for _, value in ipairs(vim.split(result.stdout or "", "\n", { plain = true, trimempty = true })) do
		if value:sub(1, 1) ~= "/" and not value:find("%.%./", 1, true) then
			values[value] = workspace .. "/" .. value
		end
	end
	return values
end

function M.candidates(opts)
	if type(opts) ~= "table" then
		fail("candidates requires options")
	end
	local workspace = root(opts.root, "root")
	local settings = opts.settings
	if type(settings) ~= "table" then
		fail("settings must be an object")
	end
	local values = { skills = {}, files = {} }
	for _, item in ipairs(skill_items(workspace, settings)) do
		table.insert(values.skills, { label = "#" .. item.name, detail = item.path })
	end
	for path in pairs(tracked(workspace)) do
		table.insert(values.files, { label = "@" .. path, detail = path })
	end
	table.sort(values.files, function(left, right)
		return left.label < right.label
	end)
	return values
end

function M.resolve(opts)
	if type(opts) ~= "table" or type(opts.prompt) ~= "string" or vim.trim(opts.prompt) == "" then
		fail("resolve requires a non-empty prompt")
	end
	local workspace = root(opts.root, "root")
	local settings = opts.settings
	if type(settings) ~= "table" then
		fail("settings must be an object")
	end
	local maximum = settings.max_file_bytes
	local remaining = settings.max_total_bytes
	local artifacts, sections, selected = {}, {}, {}
	local skills = {}
	for _, item in ipairs(skill_items(workspace, settings)) do
		skills[item.name] = skills[item.name] or item
	end
	local files = tracked(workspace)
	local function include(kind, name, path, source)
		local key = kind .. ":" .. path
		if selected[key] then
			return
		end
		if #artifacts >= settings.max_files then
			fail("reference count exceeds configured limit")
		end
		local content, reason = read(path, math.min(maximum, remaining))
		if not content then
			fail(kind .. " reference " .. name .. " " .. (reason or "is unavailable"))
		end
		local inspected = redact.inspect(content)
		if #inspected.text > remaining then
			fail("reference content exceeds configured total limit")
		end
		remaining = remaining - #inspected.text
		selected[key] = true
		table.insert(
			artifacts,
			{ kind = kind, ref = name, source = source, bytes = #inspected.text, redactions = inspected.matches }
		)
		table.insert(sections, "## Gator reference · " .. kind .. " `" .. name .. "`\n\n" .. inspected.text)
	end
	for token in opts.prompt:gmatch("#([%w_.-]+)") do
		local item = skills[token]
		if item then
			include("directive", token, item.path, item.source)
		end
	end
	for token in opts.prompt:gmatch("@([^%s]+)") do
		local path = files[token]
		if path then
			include("file", token, path, "git-tracked")
		end
	end
	return {
		prompt = opts.prompt,
		context = #sections > 0 and table.concat(sections, "\n\n") or nil,
		artifacts = artifacts,
	}
end

return M
