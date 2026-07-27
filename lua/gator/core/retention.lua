local errors = require("gator.error")
local M = {}
local Retention = {}
local Schedule = {}

Retention.__index = Retention
Schedule.__index = Schedule
M.categories = { "transcripts", "indices", "worktree_records", "telemetry" }
M.project_categories = { "transcripts", "bundles", "handoffs", "reviews", "runs", "events" }

local function fail(detail)
	errors.raise(errors.new("retention.invalid", "Local retention request is invalid", {
		detail = detail,
		remedy = "Use managed local-state paths, review the cleanup plan, then explicitly confirm deletion.",
	}))
end

local function now_seconds()
	return math.floor(vim.uv.gettimeofday().sec)
end

local function is_category(name, categories)
	return vim.tbl_contains(categories, name)
end

local function path_within(path, root)
	path = vim.fs.normalize(path)
	root = vim.fs.normalize(root)
	return path == root or path:sub(1, #root + 1) == root .. "/"
end

local function collect(root, result)
	local handle, err = vim.uv.fs_scandir(root)
	if not handle then
		if err and err:match("ENOENT") then
			return
		end
		fail("cannot scan managed path: " .. root .. ": " .. tostring(err))
	end
	while true do
		local name, kind = vim.uv.fs_scandir_next(handle)
		if not name then
			break
		end
		local path = root .. "/" .. name
		if kind == "directory" then
			collect(path, result)
		elseif kind == "file" then
			local stat = vim.uv.fs_lstat(path)
			if stat and stat.type == "file" then
				table.insert(result, { path = path, mtime = stat.mtime.sec, bytes = stat.size })
			end
		end
	end
end

function M.new(opts)
	opts = opts or {}
	if
		type(opts) ~= "table"
		or type(opts.root) ~= "string"
		or opts.root == ""
		or type(opts.paths) ~= "table"
		or type(opts.max_age) ~= "table"
	then
		fail("root, paths, and max_age must be provided")
	end
	local root = vim.fs.normalize(opts.root)
	local paths = {}
	local max_age = {}
	local categories = opts.categories or M.categories
	if type(categories) ~= "table" or not vim.islist(categories) or #categories == 0 then
		fail("categories must be a non-empty array")
	end
	for _, category in ipairs(categories) do
		if type(category) ~= "string" or category == "" then
			fail("categories must contain non-empty names")
		end
		local path = opts.paths[category]
		local age = opts.max_age[category]
		if type(path) ~= "string" or path == "" then
			fail(category .. " path must be a non-empty string")
		end
		if type(age) ~= "number" or age < 0 or age % 1 ~= 0 then
			fail(category .. " max_age must be a non-negative integer")
		end
		path = vim.fs.normalize(path)
		if not path_within(path, root) then
			fail(category .. " path must stay within retention root")
		end
		paths[category] = path
		max_age[category] = age
	end
	return setmetatable(
		{ root = root, paths = paths, max_age = max_age, categories = vim.deepcopy(categories) },
		Retention
	)
end

function M.default(max_age)
	max_age = max_age or {}
	local root = vim.fn.stdpath("state") .. "/gator"
	local paths = {}
	for _, category in ipairs(M.categories) do
		paths[category] = root .. "/" .. category
	end
	return M.new({ root = root, paths = paths, max_age = max_age })
end

function M.project(root, max_age_days)
	if type(root) ~= "string" or root == "" then
		fail("project root must be non-empty text")
	end
	if type(max_age_days) ~= "number" or max_age_days < 0 or max_age_days % 1 ~= 0 then
		fail("project max_age_days must be a non-negative integer")
	end
	local paths, max_age = {}, {}
	for _, category in ipairs(M.project_categories) do
		paths[category] = root .. "/" .. category
		max_age[category] = max_age_days * 24 * 60 * 60
	end
	return M.new({ root = root, paths = paths, max_age = max_age, categories = M.project_categories })
end

function Retention:plan(now, opts)
	now = now or now_seconds()
	if type(now) ~= "number" or now < 0 or now % 1 ~= 0 then
		fail("now must be a non-negative integer")
	end
	opts = opts or {}
	if type(opts) ~= "table" or (opts.exclude ~= nil and type(opts.exclude) ~= "function") then
		fail("plan options must provide an optional exclude callback")
	end
	local result = {}
	for _, category in ipairs(self.categories) do
		local files = {}
		collect(self.paths[category], files)
		for _, file in ipairs(files) do
			if
				now - file.mtime >= self.max_age[category] and not (opts.exclude and opts.exclude(file.path, category))
			then
				table.insert(result, { category = category, path = file.path, mtime = file.mtime, bytes = file.bytes, reason = "age" })
			end
		end
	end
	table.sort(result, function(left, right)
		return left.path < right.path
	end)
	return result
end

function Retention:inventory()
	if getmetatable(self) ~= Retention then
		fail("inventory requires a retention manager")
	end
	local items, categories, bytes = {}, {}, 0
	for _, category in ipairs(self.categories) do
		local files = {}
		collect(self.paths[category], files)
		local category_bytes = 0
		for _, file in ipairs(files) do
			local entry = { category = category, path = file.path, mtime = file.mtime, bytes = file.bytes }
			table.insert(items, entry)
			category_bytes = category_bytes + file.bytes
			bytes = bytes + file.bytes
		end
		table.insert(categories, { category = category, bytes = category_bytes, files = #files })
	end
	table.sort(items, function(left, right)
		return left.path < right.path
	end)
	return { bytes = bytes, categories = categories, items = items }
end

function Retention:quota_plan(max_bytes, opts)
	if getmetatable(self) ~= Retention then
		fail("quota_plan requires a retention manager")
	end
	if type(max_bytes) ~= "number" or max_bytes < 0 or max_bytes % 1 ~= 0 then
		fail("max_bytes must be a non-negative integer")
	end
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("quota_plan options must be an object")
	end
	for key in pairs(opts) do
		if key ~= "exclude" and key ~= "already_scheduled_bytes" then
			fail("quota_plan contains unsupported field: " .. tostring(key))
		end
	end
	if opts.exclude ~= nil and type(opts.exclude) ~= "function" then
		fail("quota_plan exclude must be a function")
	end
	local scheduled = opts.already_scheduled_bytes or 0
	if type(scheduled) ~= "number" or scheduled < 0 or scheduled % 1 ~= 0 then
		fail("already_scheduled_bytes must be a non-negative integer")
	end
	local inventory = self:inventory()
	if max_bytes == 0 then
		return {}, inventory
	end
	local remaining = math.max(0, inventory.bytes - scheduled)
	if remaining <= max_bytes then
		return {}, inventory
	end
	local candidates = {}
	for _, item in ipairs(inventory.items) do
		if not (opts.exclude and opts.exclude(item.path, item.category)) then
			table.insert(candidates, item)
		end
	end
	table.sort(candidates, function(left, right)
		return left.mtime == right.mtime and left.path < right.path or left.mtime < right.mtime
	end)
	local result = {}
	for _, item in ipairs(candidates) do
		if remaining <= max_bytes then
			break
		end
		remaining = remaining - item.bytes
		table.insert(result, {
			category = item.category,
			path = item.path,
			mtime = item.mtime,
			bytes = item.bytes,
			reason = "quota",
		})
	end
	return result, inventory
end

function Retention:prune(plan, confirm)
	if confirm ~= true then
		fail("cleanup requires explicit confirmation")
	end
	if type(plan) ~= "table" or not vim.islist(plan) then
		fail("plan must be an array returned by retention:plan")
	end
	local removed = {}
	for _, candidate in ipairs(plan) do
		if
			type(candidate) ~= "table"
			or not is_category(candidate.category, self.categories)
			or type(candidate.path) ~= "string"
			or type(candidate.mtime) ~= "number"
		then
			fail("plan contains an invalid candidate")
		end
		local root = self.paths[candidate.category]
		if not path_within(candidate.path, root) then
			fail("plan candidate escapes its managed path")
		end
		local stat = vim.uv.fs_lstat(candidate.path)
		if stat and stat.type == "file" and stat.mtime.sec == candidate.mtime then
			local ok, err = vim.uv.fs_unlink(candidate.path)
			if not ok then
				fail("cannot remove stale file: " .. candidate.path .. ": " .. tostring(err))
			end
			table.insert(removed, candidate.path)
		end
	end
	return removed
end

function Retention:schedule(opts)
	if getmetatable(self) ~= Retention or type(opts) ~= "table" then
		fail("schedule requires a retention manager and options")
	end
	for key in pairs(opts) do
		if key ~= "interval_ms" and key ~= "timer" and key ~= "now" and key ~= "on_plan" and key ~= "confirm" then
			fail("schedule contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.interval_ms) ~= "number" or opts.interval_ms < 1 or opts.interval_ms % 1 ~= 0 then
		fail("schedule interval_ms must be a positive integer")
	end
	if opts.now ~= nil and type(opts.now) ~= "function" then
		fail("schedule now must be a function")
	end
	if opts.on_plan ~= nil and type(opts.on_plan) ~= "function" then
		fail("schedule on_plan must be a function")
	end
	if opts.confirm ~= nil and type(opts.confirm) ~= "boolean" then
		fail("schedule confirm must be boolean")
	end
	local timer = opts.timer or vim.uv.new_timer()
	if
		type(timer) ~= "table"
		or type(timer.start) ~= "function"
		or type(timer.stop) ~= "function"
		or type(timer.close) ~= "function"
	then
		fail("schedule timer must expose start, stop, and close")
	end
	local value = setmetatable({ timer = timer, active = true }, Schedule)
	timer:start(
		opts.interval_ms,
		opts.interval_ms,
		vim.schedule_wrap(function()
			if not value.active then
				return
			end
			local plan = self:plan(opts.now and opts.now() or nil)
			if opts.on_plan then
				opts.on_plan(vim.deepcopy(plan))
			end
			if opts.confirm then
				self:prune(plan, true)
			end
		end)
	)
	return value
end

function Schedule:cancel()
	if getmetatable(self) ~= Schedule or not self.active then
		return false
	end
	self.active = false
	self.timer:stop()
	self.timer:close()
	return true
end

return M
