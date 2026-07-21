local task = require("gator").module("core").task
local filesystem = require("gator").module("core").filesystem
local picker = require("gator.ui").task_file_picker
local entity = task.new({
	id = "task-file-picker",
	objective = "Pick Markdown task",
	sessions = {},
	evidence = {},
	created_at = 1,
	updated_at = 1,
})
local selected
picker.open({
	files = { { path = "/fixture/task-file-picker.md", task = entity } },
	on_select = function(value)
		selected = value
	end,
})
require("gator.ui").picker.confirm()
assert(
	selected.path == "/fixture/task-file-picker.md" and selected.task.id == "task-file-picker",
	"Markdown picker must preserve canonical task identity"
)
local files = {}
local projected = picker.create({
	path = "/fixture/task-file-picker.md",
	task = entity,
	filesystem = filesystem.new({
		mkdir = function()
			return true
		end,
		write = function(path, value)
			files[path] = value
			return true
		end,
		rename = function(source, target)
			files[target], files[source] = files[source], nil
			return true
		end,
		remove = function(path)
			files[path] = nil
			return true
		end,
	}),
})
assert(
	files[projected.path] == projected.content and projected.task.id == entity.id,
	"Markdown creation must atomically project a canonical task"
)
assert(
	not pcall(picker.create, { path = "/fixture/invalid.md", task = {} }),
	"Markdown creation must reject noncanonical tasks"
)
