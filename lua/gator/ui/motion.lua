local M = {}
local defaults = { enabled = true, interval_ms = 120, reduced = false }
local settings = vim.deepcopy(defaults)

local function fail(message)
	error("Gator UI motion: " .. message, 3)
end

function M.resolve(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("settings must be an object")
	end
	for key in pairs(opts) do
		if key ~= "enabled" and key ~= "interval_ms" and key ~= "reduced" then
			fail("settings contain unsupported field: " .. tostring(key))
		end
	end
	local value = vim.tbl_deep_extend("force", vim.deepcopy(defaults), opts)
	if type(value.enabled) ~= "boolean" or type(value.reduced) ~= "boolean" then
		fail("enabled and reduced must be boolean")
	end
	if type(value.interval_ms) ~= "number" or value.interval_ms < 16 or value.interval_ms % 1 ~= 0 then
		fail("interval_ms must be an integer of at least 16")
	end
	return value
end

function M.configure(opts)
	settings = M.resolve(opts)
	return vim.deepcopy(settings)
end

function M.active(opts)
	local value = opts and M.resolve(opts) or settings
	return value.enabled and not value.reduced
end

function M.spinner(opts)
	opts = opts or {}
	if type(opts) ~= "table" then
		fail("spinner options must be an object")
	end
	for key in pairs(opts) do
		if key ~= "settings" and key ~= "defer" and key ~= "frames" then
			fail("spinner options contain unsupported field: " .. tostring(key))
		end
	end
	local value = opts.settings and M.resolve(opts.settings) or settings
	local defer = opts.defer or vim.defer_fn
	if type(defer) ~= "function" then
		fail("spinner defer must be a function")
	end
	local frames = opts.frames or { "·", "•", "●", "•" }
	if type(frames) ~= "table" or not vim.islist(frames) or #frames == 0 then
		fail("spinner frames must be a non-empty array")
	end
	for index, frame in ipairs(frames) do
		if type(frame) ~= "string" or frame == "" then
			fail("spinner frame " .. index .. " must be a non-empty string")
		end
	end
	local spinner = { active = false, frame = 1, generation = 0 }
	function spinner.stop()
		spinner.active = false
		spinner.generation = spinner.generation + 1
	end
	function spinner.start(render)
		if type(render) ~= "function" then
			fail("spinner render must be a function")
		end
		spinner.stop()
		render(frames[spinner.frame])
		if not (value.enabled and not value.reduced) then
			return spinner
		end
		spinner.active = true
		local generation = spinner.generation
		local function tick()
			if not spinner.active or spinner.generation ~= generation then
				return
			end
			spinner.frame = spinner.frame % #frames + 1
			render(frames[spinner.frame])
			defer(tick, value.interval_ms)
		end
		defer(tick, value.interval_ms)
		return spinner
	end
	return spinner
end

function M.transition(opts)
	if type(opts) ~= "table" then
		fail("transition options must be an object")
	end
	for key in pairs(opts) do
		if
			key ~= "settings"
			and key ~= "defer"
			and key ~= "from"
			and key ~= "to"
			and key ~= "steps"
			and key ~= "render"
		then
			fail("transition options contain unsupported field: " .. tostring(key))
		end
	end
	if type(opts.from) ~= "number" or type(opts.to) ~= "number" or type(opts.render) ~= "function" then
		fail("transition requires numeric from, numeric to, and render function")
	end
	if type(opts.steps) ~= "number" or opts.steps < 1 or opts.steps % 1 ~= 0 then
		fail("transition steps must be a positive integer")
	end
	local value = opts.settings and M.resolve(opts.settings) or settings
	local defer = opts.defer or vim.defer_fn
	if type(defer) ~= "function" then
		fail("transition defer must be a function")
	end
	local transition = { active = true }
	function transition.stop()
		transition.active = false
	end
	if not (value.enabled and not value.reduced) or opts.from == opts.to then
		opts.render(opts.to, true)
		transition.active = false
		return transition
	end
	local step = 0
	local function tick()
		if not transition.active then
			return
		end
		step = step + 1
		local done = step == opts.steps
		local current = done and opts.to or opts.from + (opts.to - opts.from) * step / opts.steps
		opts.render(current, done)
		if done then
			transition.active = false
			return
		end
		defer(tick, value.interval_ms)
	end
	opts.render(opts.from, false)
	defer(tick, value.interval_ms)
	return transition
end

return M
