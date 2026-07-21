local redact = require("gator.policy.redact")
local filesystem = require("gator.core.filesystem")
local task = require("gator.core.task")

local M = {
	api_version = 1,
	schema_version = 1,
	marker = "gator-task",
	scalar_encoding = "plain-or-json-string",
	title = "# Gator Task",
	headings = {
		objective = "## Objective",
		metadata = "## Metadata",
		sessions = "## Sessions",
		evidence = "## Evidence",
	},
	metadata = {
		required = { "id", "lifecycle", "created-at", "updated-at" },
		optional = { "workspace-kind", "workspace-root" },
	},
	session_fields = { "provider", "id", "owner" },
	evidence_fields = { "kind", "ref" },
}

local section_order = { "objective", "metadata", "sessions", "evidence" }

local function fail(message)
	error("Gator task file: " .. redact.text(tostring(message)), 3)
end

local function version(value)
	if type(value) ~= "number" or value % 1 ~= 0 or value ~= M.schema_version then
		fail("task-file schema version is unsupported: " .. tostring(value))
	end
	return value
end

local function lines(value)
	if type(value) ~= "string" or value == "" then
		fail("task-file document must be non-empty Markdown")
	end
	local result = vim.split(value, "\n", { plain = true })
	for index, line in ipairs(result) do
		result[index] = line:gsub("\r$", "")
	end
	return result
end

function M.specification()
	return vim.deepcopy({
		schema_version = M.schema_version,
		marker = M.marker,
		scalar_encoding = M.scalar_encoding,
		title = M.title,
		headings = M.headings,
		metadata = M.metadata,
		session_fields = M.session_fields,
		evidence_fields = M.evidence_fields,
	})
end

function M.template(value)
	if value == nil then
		value = M.schema_version
	end
	version(value)
	return table.concat({
		"---",
		M.marker .. ": " .. M.schema_version,
		"---",
		"",
		M.title,
		"",
		M.headings.objective,
		"",
		M.headings.metadata,
		"",
		M.headings.sessions,
		"",
		M.headings.evidence,
		"",
	}, "\n")
end

function M.validate_layout(value)
	local document = lines(value)
	if document[1] ~= "---" or document[3] ~= "---" then
		fail("task-file frontmatter must use the versioned delimiter")
	end
	local prefix = M.marker .. ": "
	local encoded = document[2]:sub(#prefix + 1)
	if document[2]:sub(1, #prefix) ~= prefix or not encoded:match("^%d+$") then
		fail("task-file frontmatter must declare " .. M.marker)
	end
	version(tonumber(encoded))
	if document[4] ~= "" or document[5] ~= M.title then
		fail("task-file must declare the canonical title after frontmatter")
	end
	local sections, previous = {}, 5
	for _, name in ipairs(section_order) do
		local heading = M.headings[name]
		local position
		for index = previous + 1, #document do
			if document[index] == heading then
				position = index
				break
			end
		end
		if not position then
			fail("task-file is missing " .. heading)
		end
		sections[name] = { heading = heading, first_line = position + 1 }
		previous = position
	end
	local headings = {}
	for index, line in ipairs(document) do
		for _, name in ipairs(section_order) do
			if line == M.headings[name] then
				headings[line] = (headings[line] or 0) + 1
				if headings[line] > 1 then
					fail("task-file contains duplicate section at line " .. index)
				end
			end
		end
	end
	for index, name in ipairs(section_order) do
		local next_name = section_order[index + 1]
		sections[name].last_line = next_name and sections[next_name].first_line - 2 or #document
	end
	return { schema_version = M.schema_version, sections = sections }
end

local function section_lines(document, section)
	local result = {}
	for index = section.first_line, section.last_line do
		table.insert(result, document[index])
	end
	return result
end

local function decode_scalar(value, name)
	if value:sub(1, 1) ~= '"' then
		return value
	end
	local ok, decoded = pcall(vim.json.decode, value)
	if not ok or type(decoded) ~= "string" then
		fail("task-file " .. name .. " has invalid JSON string encoding")
	end
	return decoded
end

local function metadata(value)
	local allowed, result = {}, {}
	for _, name in ipairs(M.metadata.required) do
		allowed[name] = true
	end
	for _, name in ipairs(M.metadata.optional) do
		allowed[name] = true
	end
	for _, line in ipairs(value) do
		if vim.trim(line) ~= "" then
			local name, field = line:match("^%- ([a-z][a-z-]*): (.+)$")
			if not name or not allowed[name] then
				fail("task-file metadata field is unavailable")
			end
			if result[name] then
				fail("task-file metadata field is duplicated: " .. name)
			end
			result[name] = field
		end
	end
	for _, name in ipairs(M.metadata.required) do
		if not result[name] then
			fail("task-file metadata is missing " .. name)
		end
	end
	if (result["workspace-kind"] == nil) ~= (result["workspace-root"] == nil) then
		fail("task-file workspace metadata must include kind and root together")
	end
	for _, name in ipairs({ "created-at", "updated-at" }) do
		if not result[name]:match("^%d+$") then
			fail("task-file metadata " .. name .. " must be an integer timestamp")
		end
		result[name] = tonumber(result[name])
	end
	for _, name in ipairs({ "id", "lifecycle", "workspace-kind", "workspace-root" }) do
		if result[name] then
			result[name] = decode_scalar(result[name], "metadata " .. name)
		end
	end
	return result
end

local function sessions(value)
	local result, index = {}, 1
	while index <= #value do
		if vim.trim(value[index]) == "" then
			index = index + 1
		else
			local provider = value[index]:match("^%- provider: (.+)$")
			local id = value[index + 1] and value[index + 1]:match("^  id: (.+)$")
			local owner = value[index + 2] and value[index + 2]:match("^  owner: (.+)$")
			if not provider or not id or not owner then
				fail("task-file session records must declare provider, id, and owner")
			end
			table.insert(result, {
				provider = decode_scalar(provider, "session provider"),
				id = decode_scalar(id, "session id"),
				owner = decode_scalar(owner, "session owner"),
			})
			index = index + 3
		end
	end
	return result
end

local function evidence(value)
	local result, index = {}, 1
	while index <= #value do
		if vim.trim(value[index]) == "" then
			index = index + 1
		else
			local kind = value[index]:match("^%- kind: (.+)$")
			local ref = value[index + 1] and value[index + 1]:match("^  ref: (.+)$")
			if not kind or not ref then
				fail("task-file evidence records must declare kind and ref")
			end
			table.insert(result, {
				kind = decode_scalar(kind, "evidence kind"),
				ref = decode_scalar(ref, "evidence ref"),
			})
			index = index + 2
		end
	end
	return result
end

function M.parse(value)
	local layout = M.validate_layout(value)
	local document = lines(value)
	local objective = vim.trim(table.concat(section_lines(document, layout.sections.objective), "\n"))
	if objective == "" then
		fail("task-file objective must be non-empty")
	end
	local fields = metadata(section_lines(document, layout.sections.metadata))
	local record = {
		id = fields.id,
		objective = objective,
		lifecycle = fields.lifecycle,
		created_at = fields["created-at"],
		updated_at = fields["updated-at"],
		sessions = sessions(section_lines(document, layout.sections.sessions)),
		evidence = evidence(section_lines(document, layout.sections.evidence)),
	}
	if fields["workspace-kind"] then
		record.workspace = { kind = fields["workspace-kind"], root = fields["workspace-root"] }
	end
	local ok, entity = pcall(task.from_record, record)
	if not ok then
		fail("task-file definition is invalid")
	end
	return entity
end

local function encode_scalar(value)
	if type(value) ~= "string" then
		fail("task-file scalar must be text")
	end
	if value:find("[\r\n]") or value:match("^%s") or value:match("%s$") or value:sub(1, 1) == '"' then
		return vim.json.encode(value)
	end
	return value
end

function M.render(value)
	local record = task.to_record(value)
	local document = {
		"---",
		M.marker .. ": " .. M.schema_version,
		"---",
		"",
		M.title,
		"",
		M.headings.objective,
		record.objective:gsub("\r\n", "\n"):gsub("\r", "\n"),
		"",
		M.headings.metadata,
		"- id: " .. encode_scalar(record.id),
		"- lifecycle: " .. encode_scalar(record.lifecycle),
		"- created-at: " .. record.created_at,
		"- updated-at: " .. record.updated_at,
	}
	if record.workspace then
		table.insert(document, "- workspace-kind: " .. encode_scalar(record.workspace.kind))
		table.insert(document, "- workspace-root: " .. encode_scalar(record.workspace.root))
	end
	table.insert(document, "")
	table.insert(document, M.headings.sessions)
	for _, session in ipairs(record.sessions) do
		table.insert(document, "- provider: " .. encode_scalar(session.provider))
		table.insert(document, "  id: " .. encode_scalar(session.id))
		table.insert(document, "  owner: " .. encode_scalar(session.owner))
	end
	table.insert(document, "")
	table.insert(document, M.headings.evidence)
	for _, reference in ipairs(record.evidence) do
		table.insert(document, "- kind: " .. encode_scalar(reference.kind))
		table.insert(document, "  ref: " .. encode_scalar(reference.ref))
	end
	table.insert(document, "")
	local rendered = table.concat(document, "\n")
	M.parse(rendered)
	return rendered
end

function M.write(path, value, opts)
	if type(path) ~= "string" or not path:match("%.md$") then
		fail("task-file path must be a Markdown file")
	end
	opts = opts or {}
	if type(opts) ~= "table" or (opts.filesystem ~= nil and not filesystem.is(opts.filesystem)) then
		fail("task-file write requires an optional filesystem boundary")
	end
	for key in pairs(opts) do
		if key ~= "filesystem" then
			fail("task-file write contains unsupported field: " .. tostring(key))
		end
	end
	local content = M.render(value)
	local fs = opts.filesystem or filesystem.new()
	local parent = vim.fn.fnamemodify(path, ":h")
	if not fs:mkdir(parent) then
		fail("cannot create task-file directory")
	end
	local temporary = path .. ".tmp-" .. vim.uv.hrtime()
	if not fs:write(temporary, content) then
		fs:remove(temporary)
		fail("cannot write task-file projection")
	end
	if not fs:rename(temporary, path) then
		fs:remove(temporary)
		fail("cannot replace task-file projection")
	end
	return { path = path, content = content, task = task.from_record(task.to_record(value)) }
end

return M
