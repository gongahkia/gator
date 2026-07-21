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
