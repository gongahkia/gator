local redact = require("gator.policy.redact")

local M = { schema_version = 1 }
local Store = {}
Store.__index = Store

local states = {
	starting = true,
	running = true,
	waiting_input = true,
	completed = true,
	failed = true,
	stopped = true,
	detached = true,
}

local function fail(message)
	error("Gator run store: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function identifier(value, name)
	value = text(value, name)
	if not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function json(path)
	if vim.fn.filereadable(path) == 0 then
		return nil
	end
	local ok, value = pcall(vim.json.decode, table.concat(vim.fn.readfile(path), "\n"))
	if not ok or type(value) ~= "table" then
		fail("invalid JSON: " .. path)
	end
	return value
end

local function atomic(path, value)
	local temporary = path .. ".tmp-" .. tostring(vim.uv.hrtime())
	local ok, err = pcall(vim.fn.writefile, { vim.json.encode(value) }, temporary)
	if not ok or err ~= 0 then
		fail("cannot write " .. path)
	end
	if not vim.uv.fs_rename(temporary, path) then
		pcall(vim.fn.delete, temporary)
		fail("cannot replace " .. path)
	end
end

local function resolve_git_path(root, value)
	if value:sub(1, 1) == "/" then
		return value
	end
	return root .. "/" .. value
end

local function ignore(root)
	local result = vim.system({ "git", "rev-parse", "--git-path", "info/exclude" }, { cwd = root, text = true }):wait()
	if result.code ~= 0 then
		fail("Git local exclude path is unavailable")
	end
	local path = resolve_git_path(root, vim.trim(result.stdout or ""))
	if path == root .. "/" then
		fail("Git local exclude path is empty")
	end
	local parent = vim.fn.fnamemodify(path, ":h")
	if vim.fn.mkdir(parent, "p") ~= 1 and vim.fn.isdirectory(parent) ~= 1 then
		fail("cannot create Git local exclude directory")
	end
	local lines = vim.fn.filereadable(path) == 1 and vim.fn.readfile(path) or {}
	for _, line in ipairs(lines) do
		if line == ".gator/" then
			return path
		end
	end
	table.insert(lines, ".gator/")
	if vim.fn.writefile(lines, path) ~= 0 then
		fail("cannot update Git local exclude")
	end
	return path
end

local function normalize_run(value)
	if type(value) ~= "table" then
		fail("run must be an object")
	end
	for key in pairs(value) do
		if
			not ({
				id = true,
				provider = true,
				role = true,
				transport = true,
				state = true,
				workspace = true,
				session = true,
				parent_run_id = true,
				bundle_id = true,
				objective = true,
				transcript = true,
				usage = true,
				budget = true,
				created_at = true,
				updated_at = true,
				process = true,
			})[key]
		then
			fail("run contains unsupported field: " .. tostring(key))
		end
	end
	local run = vim.deepcopy(value)
	run.id = identifier(run.id, "run.id")
	run.provider = identifier(run.provider, "run.provider")
	run.role = run.role or "primary"
	if not ({ primary = true, writer = true, reviewer = true, research = true })[run.role] then
		fail("run.role is unavailable")
	end
	if run.transport ~= "chat" and run.transport ~= "terminal" then
		fail("run.transport must be chat or terminal")
	end
	if not states[run.state] then
		fail("run.state is unavailable")
	end
	if type(run.workspace) ~= "table" or (run.workspace.kind ~= "project" and run.workspace.kind ~= "worktree") then
		fail("run.workspace must identify a project or worktree")
	end
	run.workspace.root = text(run.workspace.root, "run.workspace.root")
	if run.session ~= nil then
		if type(run.session) ~= "table" then
			fail("run.session must be an object")
		end
		run.session.id = text(run.session.id, "run.session.id")
		if type(run.session.resume_supported) ~= "boolean" then
			fail("run.session.resume_supported must be boolean")
		end
	end
	if run.parent_run_id ~= nil then
		run.parent_run_id = identifier(run.parent_run_id, "run.parent_run_id")
	end
	if run.bundle_id ~= nil then
		run.bundle_id = identifier(run.bundle_id, "run.bundle_id")
	end
	run.objective = redact.text(text(run.objective, "run.objective"))
	if run.transcript ~= "available" and run.transcript ~= "unavailable" then
		fail("run.transcript must be available or unavailable")
	end
	if type(run.usage) ~= "table" or not ({ reported = true, estimated = true, unknown = true })[run.usage.state] then
		fail("run.usage must declare reported, estimated, or unknown")
	end
	for _, field in ipairs({ "input_tokens", "output_tokens", "total_tokens", "context_tokens_estimate" }) do
		if run.usage[field] ~= nil and (type(run.usage[field]) ~= "number" or run.usage[field] < 0) then
			fail("run.usage." .. field .. " must be non-negative")
		end
	end
	run.budget = run.budget or { limit_tokens = 0, action = "warn", state = "unbounded" }
	if type(run.budget) ~= "table" then
		fail("run.budget must be an object")
	end
	if type(run.budget.limit_tokens) ~= "number" or run.budget.limit_tokens < 0 or run.budget.limit_tokens % 1 ~= 0 then
		fail("run.budget.limit_tokens must be a non-negative integer")
	end
	if run.budget.action ~= "warn" and run.budget.action ~= "stop" then
		fail("run.budget.action must be warn or stop")
	end
	if not ({ unbounded = true, unknown = true, tracking = true, exhausted = true })[run.budget.state] then
		fail("run.budget.state is unavailable")
	end
	for _, field in ipairs({ "created_at", "updated_at" }) do
		if type(run[field]) ~= "number" or run[field] < 0 or run[field] % 1 ~= 0 then
			fail("run." .. field .. " must be a non-negative integer")
		end
	end
	return run
end

function M.new(root)
	root = vim.uv.fs_realpath(text(root, "project root"))
	if not root or vim.fn.isdirectory(root) ~= 1 then
		fail("project root must resolve to a directory")
	end
	return setmetatable({
		root = root,
		directory = root .. "/.gator",
		runs_directory = root .. "/.gator/runs",
		bundles_directory = root .. "/.gator/bundles",
		handoffs_directory = root .. "/.gator/handoffs",
	}, Store)
end

function M.is(value)
	return getmetatable(value) == Store
end

function Store:ensure()
	ignore(self.root)
	for _, path in ipairs({ self.directory, self.runs_directory, self.bundles_directory, self.handoffs_directory }) do
		if vim.fn.mkdir(path, "p") ~= 1 and vim.fn.isdirectory(path) ~= 1 then
			fail("cannot create local Gator state directory")
		end
	end
	return self.directory
end

function Store:project()
	self:ensure()
	return json(self.directory .. "/project.json") or { schema_version = M.schema_version }
end

function Store:set_default_provider(provider)
	provider = identifier(provider, "provider")
	local value = self:project()
	value.schema_version, value.default_provider = M.schema_version, provider
	atomic(self.directory .. "/project.json", value)
	return provider
end

function Store:list()
	self:ensure()
	local result = {}
	for _, path in ipairs(vim.fn.glob(self.runs_directory .. "/*.json", false, true)) do
		local value = json(path)
		if value and value.schema_version == M.schema_version and type(value.run) == "table" then
			table.insert(result, normalize_run(value.run))
		else
			fail("unsupported run record: " .. path)
		end
	end
	table.sort(result, function(left, right)
		return left.created_at > right.created_at
	end)
	return result
end

function Store:get(id)
	id = identifier(id, "run id")
	self:ensure()
	local value = json(self.runs_directory .. "/" .. id .. ".json")
	return value and normalize_run(value.run) or nil
end

function Store:put(value)
	self:ensure()
	local run = normalize_run(value)
	atomic(self.runs_directory .. "/" .. run.id .. ".json", { schema_version = M.schema_version, run = run })
	return vim.deepcopy(run)
end

function Store:bundle(id, body)
	id = identifier(id, "bundle id")
	body = redact.text(text(body, "bundle body"))
	self:ensure()
	local path = self.bundles_directory .. "/" .. id .. ".md"
	if vim.fn.writefile(vim.split(body, "\n", { plain = true }), path) ~= 0 then
		fail("cannot write context bundle")
	end
	return path
end

function Store:materialize_bundle(id, body, root)
	root = vim.uv.fs_realpath(text(root, "workspace root"))
	if not root then
		fail("workspace root must resolve")
	end
	local target = M.new(root)
	target:ensure()
	return target:bundle(id, body)
end

local function relative_path(value)
	if type(value) ~= "string" or value == "" or value:find("%z") or value:sub(1, 1) == "/" then
		fail("handoff file path must be relative")
	end
	for segment in value:gmatch("[^/]+") do
		if segment == "." or segment == ".." then
			fail("handoff file path must not escape its snapshot")
		end
	end
	return value
end

local function within(root, path)
	return path == root or path:sub(1, #root + 1) == root .. "/"
end

local function write_text(path, content, error_message)
	local handle, err = vim.uv.fs_open(path, "w", 420)
	if not handle then
		fail(error_message .. ": " .. tostring(err))
	end
	local written, write_err = vim.uv.fs_write(handle, content, 0)
	vim.uv.fs_close(handle)
	if written ~= #content then
		fail(error_message .. ": " .. tostring(write_err))
	end
end

local function write_snapshot_file(root, allowed_root, path, content, materialize)
	local destination = root .. "/" .. path
	local parent = vim.fn.fnamemodify(destination, ":h")
	if vim.fn.mkdir(parent, "p") ~= 1 and vim.fn.isdirectory(parent) ~= 1 then
		fail("cannot create handoff file directory")
	end
	local resolved_parent = vim.uv.fs_realpath(parent)
	if not resolved_parent or not within(allowed_root, resolved_parent) then
		fail("handoff file path must remain within its snapshot")
	end
	local existing = vim.uv.fs_lstat(destination)
	if existing and existing.type == "link" then
		fail("handoff file path must not replace a symbolic link")
	end
	write_text(
		destination,
		content,
		materialize and "cannot apply handoff file snapshot" or "cannot write handoff file snapshot"
	)
end

local function delete_snapshot_file(root, allowed_root, path)
	local destination = root .. "/" .. path
	local existing = vim.uv.fs_lstat(destination)
	if not existing then
		return
	end
	local parent = vim.fn.fnamemodify(destination, ":h")
	local resolved_parent = vim.uv.fs_realpath(parent)
	if not resolved_parent or not within(allowed_root, resolved_parent) then
		fail("handoff file path must remain within its snapshot")
	end
	if existing.type == "link" or existing.type ~= "file" then
		fail("handoff deletion must target a regular file")
	end
	if vim.fn.delete(destination) ~= 0 then
		fail("cannot apply handoff file deletion")
	end
end

function Store:materialize_handoff(id, body, snapshot, root)
	id = identifier(id, "bundle id")
	if type(snapshot) ~= "table" or type(snapshot.files) ~= "table" then
		fail("handoff snapshot must contain files")
	end
	root = vim.uv.fs_realpath(text(root, "workspace root"))
	if not root then
		fail("workspace root must resolve")
	end
	local target = M.new(root)
	target:ensure()
	target:bundle(id, body)
	local directory = target.handoffs_directory .. "/" .. id
	local files_root = directory .. "/files"
	if vim.fn.mkdir(files_root, "p") ~= 1 and vim.fn.isdirectory(files_root) ~= 1 then
		fail("cannot create handoff snapshot directory")
	end
	local resolved_files_root = vim.uv.fs_realpath(files_root)
	if not resolved_files_root then
		fail("handoff snapshot directory must resolve")
	end
	local resolved_workspace_root = vim.uv.fs_realpath(root)
	if not resolved_workspace_root then
		fail("workspace root must resolve")
	end
	for _, file in ipairs(snapshot.files) do
		local path = relative_path(file.path)
		if file.state == "included" then
			if type(file.content) ~= "string" then
				fail("included handoff file must contain text")
			end
			write_snapshot_file(files_root, resolved_files_root, path, file.content, false)
			if file.apply_content ~= nil and type(file.apply_content) ~= "string" then
				fail("applied handoff file must contain text")
			end
			write_snapshot_file(root, resolved_workspace_root, path, file.apply_content or file.content, true)
		elseif file.state == "deleted" then
			delete_snapshot_file(root, resolved_workspace_root, path)
		end
	end
	return directory
end

function Store:transcript(id, body)
	id = identifier(id, "run id")
	body = redact.text(text(body, "transcript"))
	self:ensure()
	local directory = self.directory .. "/transcripts"
	if vim.fn.mkdir(directory, "p") ~= 1 and vim.fn.isdirectory(directory) ~= 1 then
		fail("cannot create transcript directory")
	end
	local path = directory .. "/" .. id .. ".md"
	if vim.fn.writefile(vim.split(body, "\n", { plain = true }), path) ~= 0 then
		fail("cannot write transcript")
	end
	return path
end

function Store:read_transcript(id)
	id = identifier(id, "run id")
	local path = self.directory .. "/transcripts/" .. id .. ".md"
	return vim.fn.filereadable(path) == 1 and table.concat(vim.fn.readfile(path), "\n") or nil
end

function M.id(prefix)
	prefix = prefix or "run"
	return identifier(
		prefix .. "-" .. vim.fn.sha256(tostring(vim.uv.hrtime()) .. ":" .. tostring(math.random())):sub(1, 16),
		"generated id"
	)
end

return M
