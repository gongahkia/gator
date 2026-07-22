local M = {}

local function fail(message)
	error("Gator workspace branch: " .. message, 3)
end

local function branch(value, name)
	if type(value) ~= "string" or value == "" or #value > 255 then
		fail(name .. " must be a non-empty Git branch name")
	end
	if
		value == "HEAD"
		or value == "@"
		or value:sub(1, 1) == "-"
		or value:find("..", 1, true)
		or value:find("//", 1, true)
		or value:find("@{", 1, true)
		or value:sub(-1) == "."
		or value:sub(-1) == "/"
		or value:find("[%c ~^:?*%[\\]")
	then
		fail(name .. " is not a valid Git branch name")
	end
	for component in value:gmatch("[^/]+") do
		if component:sub(1, 1) == "." or component:sub(-5) == ".lock" then
			fail(name .. " is not a valid Git branch name")
		end
	end
	return value
end

local function resolved(name, task_id, source)
	return { name = name, branch = name, worktree_name = name, task_id = task_id, source = source }
end

function M.resolve(opts)
	if type(opts) ~= "table" then
		fail("resolve requires options")
	end
	for key in pairs(opts) do
		if key ~= "task_id" and key ~= "prefix" and key ~= "override" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	if type(opts.task_id) ~= "string" or not opts.task_id:match("^[a-z][a-z0-9_-]*$") then
		fail("task_id must be a lowercase identifier")
	end
	if opts.override ~= nil then
		return resolved(branch(opts.override, "override"), opts.task_id, "override")
	end
	local prefix = opts.prefix or "gator"
	if type(prefix) ~= "string" or prefix == "" then
		fail("prefix must be a non-empty string")
	end
	return resolved(branch(prefix .. "/" .. opts.task_id, "derived branch"), opts.task_id, "derived")
end

return M
