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
local review_states = {
	pending = true,
	passed = true,
	failed = true,
	accepted = true,
	changes_requested = true,
	handoff = true,
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
				review = true,
				runbook_id = true,
				depends_on = true,
			})[key]
		then
			fail("run contains unsupported field: " .. tostring(key))
		end
	end
	local run = vim.deepcopy(value)
	run.id = identifier(run.id, "run.id")
	run.provider = identifier(run.provider, "run.provider")
	run.role = run.role or "primary"
	if run.role == "research" then
		run.role = "researcher"
	end
	if not ({ primary = true, writer = true, reviewer = true, researcher = true, integrator = true })[run.role] then
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
		if run.session.path ~= nil then
			run.session.path = text(run.session.path, "run.session.path")
		end
		if run.session.provider ~= nil then
			run.session.provider = identifier(run.session.provider, "run.session.provider")
		end
		if run.session.owner ~= nil and run.session.owner ~= "provider" and run.session.owner ~= "gator" then
			fail("run.session.owner must be provider or gator")
		end
		if run.session.mode ~= nil and (type(run.session.mode) ~= "string" or run.session.mode == "") then
			fail("run.session.mode must be non-empty text")
		end
		if run.session.capabilities ~= nil then
			if
				type(run.session.capabilities) ~= "table"
				or (vim.islist(run.session.capabilities) and next(run.session.capabilities) ~= nil)
			then
				fail("run.session.capabilities must be an object")
			end
			for key, value in pairs(run.session.capabilities) do
				if type(key) ~= "string" or type(value) ~= "boolean" then
					fail("run.session.capabilities must map names to booleans")
				end
			end
		end
	end
	if run.parent_run_id ~= nil then
		run.parent_run_id = identifier(run.parent_run_id, "run.parent_run_id")
	end
	if run.bundle_id ~= nil then
		run.bundle_id = identifier(run.bundle_id, "run.bundle_id")
	end
	if run.runbook_id ~= nil then
		run.runbook_id = identifier(run.runbook_id, "run.runbook_id")
	end
	if run.depends_on ~= nil then
		if type(run.depends_on) ~= "table" or not vim.islist(run.depends_on) then
			fail("run.depends_on must be an array")
		end
		local seen = {}
		for index, value in ipairs(run.depends_on) do
			value = identifier(value, "run.depends_on[" .. index .. "]")
			if value == run.id or seen[value] then
				fail("run.depends_on must contain unique external run ids")
			end
			seen[value] = true
			run.depends_on[index] = value
		end
	end
	if run.review ~= nil then
		if type(run.review) ~= "table" then
			fail("run.review must be an object")
		end
		for key in pairs(run.review) do
			if key ~= "id" and key ~= "state" then
				fail("run.review contains unsupported field: " .. tostring(key))
			end
		end
		run.review.id = identifier(run.review.id, "run.review.id")
		if not review_states[run.review.state] then
			fail("run.review.state is unavailable")
		end
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
		reviews_directory = root .. "/.gator/reviews",
		runbooks_directory = root .. "/.gator/runbooks",
	}, Store)
end

function M.is(value)
	return getmetatable(value) == Store
end

function Store:ensure()
	ignore(self.root)
	for _, path in ipairs({
		self.directory,
		self.runs_directory,
		self.bundles_directory,
		self.handoffs_directory,
		self.reviews_directory,
		self.runbooks_directory,
	}) do
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

function Store:read_bundle(id)
	id = identifier(id, "bundle id")
	self:ensure()
	local path = self.bundles_directory .. "/" .. id .. ".md"
	if vim.fn.filereadable(path) ~= 1 then
		return nil
	end
	return table.concat(vim.fn.readfile(path), "\n")
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

local function git(root, argv)
	local result = vim.system(argv, { cwd = root, text = false }):wait()
	return result.code == 0 and (result.stdout or "") or nil
end

local function dirty_paths(root)
	local output = git(root, { "git", "status", "--porcelain=v1", "-z", "--untracked-files=all" })
	if output == nil then
		fail("Git target status is unavailable")
	end
	local result, values, index = {}, vim.split(output, "\0", { plain = true, trimempty = true }), 1
	while index <= #values do
		local value = values[index]
		if #value >= 4 then
			local path = value:sub(4)
			if relative_path(path) then
				result[path] = value:sub(1, 2)
			end
			if value:sub(1, 1) == "R" or value:sub(1, 1) == "C" or value:sub(2, 2) == "R" or value:sub(2, 2) == "C" then
				local source = values[index + 1]
				if source and relative_path(source) then
					result[source] = value:sub(1, 2)
				end
				index = index + 1
			end
		end
		index = index + 1
	end
	return result
end

local function file_hash(path)
	local stat = vim.uv.fs_lstat(path)
	if not stat then
		return nil, "missing"
	end
	if stat.type == "link" then
		return nil, "symbolic link"
	end
	if stat.type ~= "file" then
		return nil, "not a regular file"
	end
	local handle = vim.uv.fs_open(path, "r", 420)
	if not handle then
		return nil, "unreadable"
	end
	local value = vim.uv.fs_read(handle, stat.size, 0)
	vim.uv.fs_close(handle)
	if type(value) ~= "string" then
		return nil, "unreadable"
	end
	return vim.fn.sha256(value), nil, value
end

local function sanitized_snapshot(snapshot)
	local value = vim.deepcopy(snapshot)
	value.root = nil
	for _, file in ipairs(value.files or {}) do
		file.apply_content = nil
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

local function conflict_path(result, seen, path, reason, file)
	if seen[path] then
		return
	end
	seen[path] = true
	table.insert(result, {
		path = path,
		reason = reason,
		source_sha256 = file.content_sha256,
	})
end

function Store:handoff_conflicts(snapshot, root)
	if type(snapshot) ~= "table" or type(snapshot.files) ~= "table" then
		fail("handoff snapshot must contain files")
	end
	root = vim.uv.fs_realpath(text(root, "workspace root"))
	if not root then
		fail("workspace root must resolve")
	end
	local dirty = dirty_paths(root)
	local target_head = git(root, { "git", "rev-parse", "--verify", "HEAD" })
	target_head = target_head and vim.trim(target_head) or nil
	local source_head = type(snapshot.base) == "table" and snapshot.base.head or nil
	local result, seen = {}, {}
	for _, file in ipairs(snapshot.files) do
		local path = relative_path(file.path)
		if file.state == "included" or file.state == "deleted" then
			if type(source_head) == "string" and source_head ~= "" and target_head ~= source_head then
				conflict_path(result, seen, path, "target base SHA differs from source snapshot", file)
			end
			if dirty[path] then
				conflict_path(result, seen, path, "target path is modified or untracked (" .. dirty[path] .. ")", file)
			end
			if file.rename_from then
				local source_path = relative_path(file.rename_from)
				if dirty[source_path] then
					conflict_path(
						result,
						seen,
						path,
						"rename source is modified or untracked (" .. dirty[source_path] .. ")",
						file
					)
				end
			end
			local _, reason = file_hash(root .. "/" .. path)
			if reason == "symbolic link" then
				conflict_path(result, seen, path, "target path is a symbolic link", file)
			end
		end
	end
	table.sort(result, function(left, right)
		return left.path < right.path
	end)
	return result
end

function Store:preview_handoff(snapshot, root)
	if type(snapshot) ~= "table" or type(snapshot.files) ~= "table" then
		fail("handoff snapshot must contain files")
	end
	root = vim.uv.fs_realpath(text(root, "workspace root"))
	if not root then
		fail("workspace root must resolve")
	end
	local lines = {}
	for _, file in ipairs(snapshot.files) do
		if file.state == "included" or file.state == "deleted" then
			local path = relative_path(file.path)
			local _, _, before = file_hash(root .. "/" .. path)
			local after = file.state == "included" and file.apply_content or ""
			if type(after) ~= "string" then
				fail("included handoff file must contain text")
			end
			before = before or ""
			if before ~= after then
				local diff = vim.diff(before, after, { result_type = "unified", ctxlen = 3 })
				table.insert(lines, "diff --gator a/" .. redact.text(path) .. " b/" .. redact.text(path))
				if file.rename_from then
					table.insert(lines, "rename from " .. redact.text(file.rename_from))
					table.insert(lines, "rename to " .. redact.text(path))
				end
				vim.list_extend(lines, vim.split(redact.text(diff), "\n", { plain = true, trimempty = false }))
			end
		end
	end
	return #lines > 0 and table.concat(lines, "\n") or "No target-worktree changes would be applied."
end

local function decisions(value)
	if value == nil then
		return {}
	end
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail("handoff decisions must be an object")
	end
	local result = {}
	for path, choice in pairs(value) do
		path = relative_path(path)
		if choice ~= "apply" and choice ~= "skip" then
			fail("handoff decision must be apply or skip")
		end
		result[path] = choice
	end
	return result
end

function Store:materialize_handoff(id, body, snapshot, root, opts)
	id = identifier(id, "bundle id")
	if type(snapshot) ~= "table" or type(snapshot.files) ~= "table" then
		fail("handoff snapshot must contain files")
	end
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("handoff materialization options must be an object")
	end
	for key in pairs(opts) do
		if key ~= "decisions" and key ~= "apply" then
			fail("handoff materialization option is unsupported: " .. tostring(key))
		end
	end
	local selected = decisions(opts.decisions)
	if opts.apply ~= nil and type(opts.apply) ~= "boolean" then
		fail("handoff apply must be boolean")
	end
	local apply = opts.apply ~= false
	root = vim.uv.fs_realpath(text(root, "workspace root"))
	if not root then
		fail("workspace root must resolve")
	end
	local conflicts = apply and self:handoff_conflicts(snapshot, root) or {}
	for _, conflict in ipairs(conflicts) do
		if not selected[conflict.path] then
			fail("handoff target collision requires an explicit decision: " .. conflict.path)
		end
	end
	local target = M.new(root)
	target:ensure()
	local directory = target.handoffs_directory .. "/" .. id
	if vim.uv.fs_stat(directory) then
		fail("handoff artifact already exists")
	end
	target:bundle(id, body)
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
	local manifest = {
		schema_version = 1,
		bundle_id = id,
		snapshot = sanitized_snapshot(snapshot),
		decisions = selected,
		conflicts = conflicts,
	}
	manifest.sha256 = vim.fn.sha256(vim.json.encode(manifest.snapshot))
	atomic(directory .. "/manifest.json", manifest)
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
			local applied = file.apply_content or file.content
			if file.content_sha256 and vim.fn.sha256(applied) ~= file.content_sha256 then
				fail("handoff file content hash does not match manifest: " .. path)
			end
			if apply and selected[path] ~= "skip" then
				if file.rename_from and file.kind ~= "copy" then
					delete_snapshot_file(root, resolved_workspace_root, relative_path(file.rename_from))
				end
				write_snapshot_file(root, resolved_workspace_root, path, applied, true)
				local mode = type(file.modes) == "table" and file.modes.worktree or nil
				local numeric = type(mode) == "string" and tonumber(mode, 8) or nil
				if numeric then
					vim.uv.fs_chmod(root .. "/" .. path, numeric % 4096)
				end
			end
		elseif file.state == "deleted" then
			if apply and selected[path] ~= "skip" then
				delete_snapshot_file(root, resolved_workspace_root, path)
			end
		end
	end
	return directory
end

local function review_record(value)
	if type(value) ~= "table" then
		fail("review must be an object")
	end
	for key in pairs(value) do
		if
			key ~= "id"
			and key ~= "run_id"
			and key ~= "state"
			and key ~= "base_sha"
			and key ~= "diff_sha256"
			and key ~= "command_id"
			and key ~= "argv"
			and key ~= "exit_code"
			and key ~= "passed"
			and key ~= "decision"
			and key ~= "created_at"
		then
			fail("review contains unsupported field: " .. tostring(key))
		end
	end
	local record = vim.deepcopy(value)
	record.id = identifier(record.id, "review.id")
	record.run_id = identifier(record.run_id, "review.run_id")
	if not review_states[record.state] then
		fail("review.state is unavailable")
	end
	record.base_sha = text(record.base_sha, "review.base_sha")
	record.diff_sha256 = text(record.diff_sha256, "review.diff_sha256")
	if record.command_id ~= nil then
		record.command_id = identifier(record.command_id, "review.command_id")
		if type(record.argv) ~= "table" or not vim.islist(record.argv) or #record.argv == 0 then
			fail("review.argv must be a non-empty array when command_id is set")
		end
		for index, argument in ipairs(record.argv) do
			record.argv[index] = text(argument, "review.argv[" .. index .. "]")
		end
		if type(record.exit_code) ~= "number" or record.exit_code % 1 ~= 0 or type(record.passed) ~= "boolean" then
			fail("review command evidence must include an integer exit_code and passed boolean")
		end
	elseif record.argv ~= nil or record.exit_code ~= nil or record.passed ~= nil then
		fail("review command fields require command_id")
	end
	if
		record.decision ~= nil
		and record.decision ~= "accepted"
		and record.decision ~= "changes_requested"
		and record.decision ~= "handoff"
	then
		fail("review.decision is unavailable")
	end
	if type(record.created_at) ~= "number" or record.created_at < 0 or record.created_at % 1 ~= 0 then
		fail("review.created_at must be a non-negative integer")
	end
	return record
end

function Store:write_review(value, output)
	local record = review_record(value)
	if output ~= nil and type(output) ~= "string" then
		fail("review output must be text")
	end
	self:ensure()
	local directory = self.reviews_directory .. "/" .. record.run_id
	if vim.fn.mkdir(directory, "p") ~= 1 and vim.fn.isdirectory(directory) ~= 1 then
		fail("cannot create review directory")
	end
	local path = directory .. "/" .. record.id .. ".json"
	if vim.uv.fs_stat(path) then
		fail("review artifact already exists")
	end
	local output_path = directory .. "/" .. record.id .. ".log"
	local retained = redact.text(output or "")
	if vim.fn.writefile(vim.split(retained, "\n", { plain = true, trimempty = false }), output_path) ~= 0 then
		fail("cannot write review output")
	end
	local document =
		{ schema_version = 1, review = record, output_ref = "reviews/" .. record.run_id .. "/" .. record.id .. ".log" }
	atomic(path, document)
	return vim.deepcopy(document)
end

function Store:list_reviews(run_id)
	run_id = identifier(run_id, "run id")
	self:ensure()
	local directory = self.reviews_directory .. "/" .. run_id
	local result = {}
	for _, path in ipairs(vim.fn.glob(directory .. "/*.json", false, true)) do
		local document = json(path)
		if not document or document.schema_version ~= 1 or type(document.review) ~= "table" then
			fail("invalid review artifact: " .. path)
		end
		local record = review_record(document.review)
		if record.run_id ~= run_id or type(document.output_ref) ~= "string" then
			fail("invalid review artifact: " .. path)
		end
		record.output_ref = document.output_ref
		table.insert(result, record)
	end
	table.sort(result, function(left, right)
		return left.created_at > right.created_at
	end)
	return result
end

local runbook_roles = { researcher = true, writer = true, reviewer = true, integrator = true }

local function runbook_record(value)
	if type(value) ~= "table" then
		fail("runbook must be an object")
	end
	for key in pairs(value) do
		if
			key ~= "id"
			and key ~= "title"
			and key ~= "max_concurrent"
			and key ~= "max_tokens"
			and key ~= "steps"
			and key ~= "created_at"
		then
			fail("runbook contains unsupported field: " .. tostring(key))
		end
	end
	local record = vim.deepcopy(value)
	record.id = identifier(record.id, "runbook.id")
	record.title = text(record.title, "runbook.title")
	for _, field in ipairs({ "max_concurrent", "max_tokens", "created_at" }) do
		if type(record[field]) ~= "number" or record[field] < 0 or record[field] % 1 ~= 0 then
			fail("runbook." .. field .. " must be a non-negative integer")
		end
	end
	if type(record.steps) ~= "table" or not vim.islist(record.steps) or #record.steps == 0 then
		fail("runbook.steps must be a non-empty array")
	end
	local by_id = {}
	for index, step in ipairs(record.steps) do
		if type(step) ~= "table" then
			fail("runbook.steps[" .. index .. "] must be an object")
		end
		for key in pairs(step) do
			if
				key ~= "id"
				and key ~= "role"
				and key ~= "objective"
				and key ~= "provider"
				and key ~= "depends_on"
				and key ~= "run_id"
			then
				fail("runbook step contains unsupported field: " .. tostring(key))
			end
		end
		step.id = identifier(step.id, "runbook.steps[" .. index .. "].id")
		if by_id[step.id] then
			fail("runbook steps must have unique ids")
		end
		by_id[step.id] = step
		if step.role == "research" then
			step.role = "researcher"
		end
		if not runbook_roles[step.role] then
			fail("runbook step role is unavailable")
		end
		step.objective = text(step.objective, "runbook.steps[" .. index .. "].objective")
		if step.provider ~= nil then
			step.provider = identifier(step.provider, "runbook step provider")
		end
		if type(step.depends_on) ~= "table" or not vim.islist(step.depends_on) then
			fail("runbook step dependencies must be an array")
		end
		local seen = {}
		for dependency_index, dependency in ipairs(step.depends_on) do
			dependency = identifier(dependency, "runbook dependency")
			if dependency == step.id or seen[dependency] then
				fail("runbook step dependencies must be unique external step ids")
			end
			seen[dependency] = true
			step.depends_on[dependency_index] = dependency
		end
		if step.run_id ~= nil then
			step.run_id = identifier(step.run_id, "runbook step run_id")
		end
	end
	for _, step in ipairs(record.steps) do
		for _, dependency in ipairs(step.depends_on) do
			if not by_id[dependency] then
				fail("runbook dependency is unavailable: " .. dependency)
			end
		end
	end
	local visiting, visited = {}, {}
	local function walk(step)
		if visiting[step.id] then
			fail("runbook dependencies must be acyclic")
		end
		if visited[step.id] then
			return
		end
		visiting[step.id] = true
		for _, dependency in ipairs(step.depends_on) do
			walk(by_id[dependency])
		end
		visiting[step.id], visited[step.id] = nil, true
	end
	for _, step in ipairs(record.steps) do
		walk(step)
	end
	return record
end

function Store:create_runbook(value)
	local record = runbook_record(value)
	self:ensure()
	local path = self.runbooks_directory .. "/" .. record.id .. ".json"
	if vim.uv.fs_stat(path) then
		fail("runbook already exists")
	end
	atomic(path, { schema_version = 1, runbook = record })
	return vim.deepcopy(record)
end

function Store:runbook(id)
	id = identifier(id, "runbook id")
	self:ensure()
	local document = json(self.runbooks_directory .. "/" .. id .. ".json")
	if not document then
		return nil
	end
	if document.schema_version ~= 1 or type(document.runbook) ~= "table" then
		fail("invalid runbook artifact")
	end
	return runbook_record(document.runbook)
end

function Store:update_runbook(value)
	local record = runbook_record(value)
	self:ensure()
	local path = self.runbooks_directory .. "/" .. record.id .. ".json"
	if not vim.uv.fs_stat(path) then
		fail("runbook is unavailable")
	end
	atomic(path, { schema_version = 1, runbook = record })
	return vim.deepcopy(record)
end

function Store:list_runbooks()
	self:ensure()
	local result = {}
	for _, path in ipairs(vim.fn.glob(self.runbooks_directory .. "/*.json", false, true)) do
		local document = json(path)
		if not document or document.schema_version ~= 1 or type(document.runbook) ~= "table" then
			fail("invalid runbook artifact: " .. path)
		end
		table.insert(result, runbook_record(document.runbook))
	end
	table.sort(result, function(left, right)
		return left.created_at > right.created_at
	end)
	return result
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
