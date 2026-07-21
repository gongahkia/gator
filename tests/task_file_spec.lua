local format = require("gator").module("core").task_file

local specification = format.specification()
assert(
	specification.schema_version == 1
		and specification.marker == "gator-task"
		and vim.deep_equal(specification.metadata.required, { "id", "lifecycle", "created-at", "updated-at" }),
	"task-file format must publish the versioned marker and canonical metadata fields"
)
specification.metadata.required[1] = "changed"
assert(
	format.specification().metadata.required[1] == "id",
	"task-file specifications must not expose mutable schema state"
)

local template = format.template()
local layout = format.validate_layout(template)
assert(
	layout.schema_version == 1
		and layout.sections.objective.first_line < layout.sections.metadata.first_line
		and layout.sections.sessions.first_line < layout.sections.evidence.first_line,
	"task-file templates must provide ordered versioned sections"
)

local unsupported = template:gsub("gator%-task: 1", "gator-task: 2", 1)
assert(not pcall(format.validate_layout, unsupported), "task-file formats must reject unavailable schema versions")
local missing = template:gsub("## Sessions\n\n", "", 1)
assert(not pcall(format.validate_layout, missing), "task-file formats must reject missing canonical sections")

local definition = table.concat({
	"---",
	"gator-task: 1",
	"---",
	"",
	"# Gator Task",
	"",
	"## Objective",
	"Persist a user-authored task definition.",
	"",
	"## Metadata",
	"- id: task-markdown",
	"- lifecycle: planned",
	"- created-at: 1",
	"- updated-at: 2",
	"- workspace-kind: project",
	"- workspace-root: /workspace/gator",
	"",
	"## Sessions",
	"- provider: codex",
	"  id: native-markdown",
	"  owner: provider",
	"",
	"## Evidence",
	"- kind: test",
	"  ref: token: private-value",
	"",
}, "\n")
local parsed = format.parse(definition)
assert(
	parsed.id == "task-markdown"
		and parsed.workspace.root == "/workspace/gator"
		and parsed.sessions[1].owner == "provider"
		and parsed.evidence[1].ref:find("private%-value") == nil,
	"task-file parsers must preserve canonical fields while redacting durable evidence"
)
local invalid = definition:gsub("  owner: provider", "  owner: gator", 1)
assert(not pcall(format.parse, invalid), "task-file parsers must reject non-provider-owned sessions")

local filesystem = require("gator").module("core").filesystem
local files = {}
local projected = format.write("/fixture/task-markdown.md", parsed, {
	filesystem = filesystem.new({
		readable = function(path)
			return files[path] ~= nil
		end,
		read = function(path)
			return files[path]
		end,
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
local round_trip = format.parse(projected.content)
assert(
	files[projected.path] == projected.content
		and round_trip.id == parsed.id
		and round_trip.sessions[1].id == "native-markdown"
		and round_trip.evidence[1].ref:find("private%-value") == nil,
	"task-file projections must atomically preserve canonical records and redacted evidence"
)
assert(
	not pcall(format.write, "/fixture/task.txt", parsed),
	"task-file projections must reject non-Markdown destinations"
)
assert(not pcall(format.write, "/fixture/task-write-failure.md", parsed, {
	filesystem = filesystem.new({
		mkdir = function()
			return true
		end,
		write = function()
			return false
		end,
	}),
}), "task-file projections must expose filesystem write failures")

local watched, callback, stopped = {}, nil, false
files["/fixture/task-external.md"] = definition
local watcher = format.watch("/fixture/task-external.md", {
	filesystem = filesystem.new({
		readable = function(path)
			return files[path] ~= nil
		end,
		read = function(path)
			return files[path]
		end,
	}),
	backend = {
		put_task = function(_, value)
			watched[#watched + 1] = value
			return value
		end,
	},
	watch = function(_, value)
		callback = value
		return function()
			stopped = true
		end
	end,
})
files["/fixture/task-external.md"] = definition:gsub("- lifecycle: planned", "- lifecycle: running", 1)
assert(
	watcher:status().active
		and watched[1].lifecycle == "planned"
		and callback(nil, "task-external.md").lifecycle == "running"
		and watched[2].lifecycle == "running",
	"task-file watchers must import external validated changes through the storage backend"
)
files["/fixture/task-external.md"] = "invalid Markdown"
assert(
	callback(nil, "task-external.md") == false and watcher:status().last_error ~= nil,
	"task-file watchers must retain explicit errors for malformed external files"
)
assert(
	watcher:stop() and stopped and callback(nil, "task-external.md") == false,
	"task-file watchers must be cancellable"
)
