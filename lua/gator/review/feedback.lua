local run = require("gator.core.run")
local session = require("gator.core.session")
local M = {}
local decisions = { accepted = true, rejected = true }

local function fail(message)
	error("Gator review feedback: " .. message, 3)
end

local function path(value, name)
	if type(value) ~= "string" or value == "" or value:sub(1, 1) == "/" or value:find("\0", 1, true) then
		fail(name .. " must be a non-empty relative path")
	end
	for component in value:gmatch("[^/]+") do
		if component == ".." then
			fail(name .. " must not leave the workspace")
		end
	end
	return value
end

local function feedback(value)
	if type(value) ~= "table" or not vim.islist(value) or #value == 0 then
		fail("feedback must be a non-empty array")
	end
	local result = {}
	for index, item in ipairs(value) do
		if type(item) ~= "table" then
			fail("feedback " .. index .. " must be a table")
		end
		for key in pairs(item) do
			if key ~= "path" and key ~= "hunk_id" and key ~= "decision" and key ~= "annotation" then
				fail("feedback " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		if type(item.hunk_id) ~= "string" or not item.hunk_id:match("^[a-z][a-z0-9_-]*$") then
			fail("feedback " .. index .. " hunk_id must be a lowercase identifier")
		end
		if type(item.decision) ~= "string" or not decisions[item.decision] then
			fail("feedback " .. index .. " decision must be accepted or rejected")
		end
		if item.annotation ~= nil and (type(item.annotation) ~= "string" or item.annotation == "") then
			fail("feedback " .. index .. " annotation must be a non-empty string")
		end
		result[index] = {
			path = path(item.path, "feedback " .. index .. " path"),
			hunk_id = item.hunk_id,
			decision = item.decision,
			annotation = item.annotation,
		}
	end
	return result
end

function M.route(opts)
	if
		type(opts) ~= "table"
		or not run.is(opts.run)
		or not session.is(opts.session)
		or type(opts.send) ~= "function"
	then
		fail("route requires a Gator run, linked session, feedback, and send callback")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "session" and key ~= "feedback" and key ~= "send" then
			fail("route contains unsupported field: " .. tostring(key))
		end
	end
	if
		opts.run.task_id ~= opts.session.task_id
		or opts.run.provider.name ~= opts.session.provider
		or opts.run.provider.session_id == nil
		or opts.run.provider.session_id ~= opts.session.id
	then
		fail("session must be the provider-owned session linked to the run")
	end
	local selected = feedback(opts.feedback)
	local files, seen = {}, {}
	for _, item in ipairs(selected) do
		if not seen[item.path] then
			seen[item.path] = true
			table.insert(files, item.path)
		end
	end
	table.sort(files)
	local payload = {
		kind = "review.feedback",
		task_id = opts.run.task_id,
		run_id = opts.run.id,
		context = { workspace = vim.deepcopy(opts.run.workspace), files = files },
		feedback = selected,
	}
	local ok, sent = pcall(opts.send, session.reference(opts.session), vim.deepcopy(payload))
	if not ok or sent ~= true then
		fail("provider feedback routing failed")
	end
	return payload
end

return M
