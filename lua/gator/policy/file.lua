local overlay = require("gator.policy.overlay")
local M = {}

local function fail(message)
	error("Gator file policy: " .. message, 3)
end

local function relative_path(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty relative path")
	end
	if value:sub(1, 1) == "/" or value:sub(-1) == "/" or value:find("\\", 1, true) then
		fail(name .. " must be a normalized relative path")
	end
	local segments = {}
	for segment in value:gmatch("[^/]+") do
		if segment == "." or segment == ".." then
			fail(name .. " must not contain traversal segments")
		end
		table.insert(segments, segment)
	end
	if #segments == 0 or table.concat(segments, "/") ~= value then
		fail(name .. " must be a normalized relative path")
	end
	return segments
end

local function pattern(value)
	local segments = relative_path(value, "pattern")
	local previous_globstar = false
	for _, segment in ipairs(segments) do
		if segment:find("[?%[%]{}]") then
			fail("pattern uses unsupported glob syntax")
		end
		if segment:find("%*%*") and segment ~= "**" then
			fail("pattern globstar must be a complete path segment")
		end
		if segment == "**" and previous_globstar then
			fail("pattern must not repeat globstar segments")
		end
		previous_globstar = segment == "**"
	end
	return segments
end

local function segment_matches(pattern_segment, path_segment)
	local expression = "^" .. vim.pesc(pattern_segment):gsub("%%%*", "[^/]*") .. "$"
	return path_segment:match(expression) ~= nil
end

local function matches(pattern_segments, path_segments, pattern_index, path_index)
	while pattern_index <= #pattern_segments do
		local current = pattern_segments[pattern_index]
		if current == "**" then
			if pattern_index == #pattern_segments then
				return true
			end
			for candidate = path_index, #path_segments + 1 do
				if matches(pattern_segments, path_segments, pattern_index + 1, candidate) then
					return true
				end
			end
			return false
		end
		if path_index > #path_segments or not segment_matches(current, path_segments[path_index]) then
			return false
		end
		pattern_index = pattern_index + 1
		path_index = path_index + 1
	end
	return path_index > #path_segments
end

local function map(value)
	return type(value) == "table" and (not vim.islist(value) or next(value) == nil)
end

local function merge(destination, source)
	for key, value in pairs(source) do
		if map(destination[key]) and map(value) then
			merge(destination[key], value)
		else
			destination[key] = vim.deepcopy(value)
		end
	end
end

function M.matches(target, path)
	return matches(pattern(target), relative_path(path, "path"), 1, 1)
end

function M.match(opts)
	if type(opts) ~= "table" then
		fail("match requires options")
	end
	for key in pairs(opts) do
		if key ~= "path" and key ~= "overlays" then
			fail("match contains unsupported field: " .. tostring(key))
		end
	end
	local path = opts.path
	local path_segments = relative_path(path, "path")
	if type(opts.overlays) ~= "table" or not vim.islist(opts.overlays) then
		fail("overlays must be a list")
	end
	local rules = {}
	local matched = {}
	for index, value in ipairs(opts.overlays) do
		if not overlay.is(value) or value.scope ~= "file" then
			fail("overlays[" .. index .. "] must be a file policy overlay")
		end
		if matches(pattern(value.target), path_segments, 1, 1) then
			merge(rules, value.rules)
			table.insert(matched, {
				index = index,
				pattern = value.target,
				rules = vim.deepcopy(value.rules),
				provenance = vim.deepcopy(value.provenance),
			})
		end
	end
	return { path = path, rules = rules, matched = matched }
end

return M
