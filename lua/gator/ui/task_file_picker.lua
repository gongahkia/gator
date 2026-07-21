local picker = require("gator.ui.picker")
local task = require("gator.core.task")
local task_file = require("gator.core.task_file")
local M = {}

local function fail(message)
	error("Gator task-file picker: " .. message, 3)
end

function M.open(opts)
	if type(opts) ~= "table" or type(opts.on_select) ~= "function" then
		fail("open requires task files and an on_select callback")
	end
	for key in pairs(opts) do
		if key ~= "files" and key ~= "on_select" and key ~= "on_cancel" then
			fail("open contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.files) ~= "table" or not vim.islist(opts.files) then
		fail("files must be an array")
	end
	local files, items = {}, {}
	for index, value in ipairs(opts.files) do
		if type(value) ~= "table" or type(value.path) ~= "string" or value.path == "" or not task.is(value.task) then
			fail("file " .. index .. " must provide a path and canonical task")
		end
		local record = task.to_record(value.task)
		files[record.id] = { path = value.path, task = record }
		items[index] = { id = record.id, label = record.id .. " · " .. record.objective }
	end
	return picker.open({
		title = "Gator Markdown tasks",
		items = items,
		on_select = function(item)
			opts.on_select(vim.deepcopy(files[item.id]))
		end,
		on_cancel = opts.on_cancel,
	})
end

function M.create(opts)
	if type(opts) ~= "table" or type(opts.path) ~= "string" or opts.path == "" or not task.is(opts.task) then
		fail("create requires a Markdown path and canonical task")
	end
	for key in pairs(opts) do
		if key ~= "path" and key ~= "task" and key ~= "filesystem" then
			fail("create contains unsupported field: " .. tostring(key))
		end
	end
	return task_file.write(opts.path, opts.task, { filesystem = opts.filesystem })
end

return M
