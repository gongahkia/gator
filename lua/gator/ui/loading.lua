local presets = require("gator.ui.loading_presets")

local M = {}

local defaults = { enabled = true, spinner = "rattles.braille.dots", interval_ms = 0 }
local settings = vim.tbl_extend("force", vim.deepcopy(defaults), { animate = true })
local panels, sequence = {}, 0

local function fail(message)
	error("Gator loading: " .. message, 3)
end

local function resolve(value)
	if value == nil then
		value = {}
	end
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail("settings must be an object")
	end
	for key in pairs(value) do
		if key ~= "enabled" and key ~= "spinner" and key ~= "interval_ms" then
			fail("settings contain unsupported field: " .. tostring(key))
		end
	end
	local result = vim.tbl_deep_extend("force", vim.deepcopy(defaults), value)
	if type(result.enabled) ~= "boolean" then
		fail("enabled must be boolean")
	end
	if type(result.spinner) ~= "string" or not presets.has(result.spinner) then
		fail("spinner must name a bundled Rattles or Whirly preset")
	end
	if type(result.interval_ms) ~= "number" or result.interval_ms % 1 ~= 0 or result.interval_ms < 0 then
		fail("interval_ms must be a non-negative integer")
	end
	if result.interval_ms > 0 and result.interval_ms < 16 then
		fail("interval_ms must be 0 or at least 16")
	end
	return result
end

local function panel_valid(panel)
	return not panel.closed and vim.api.nvim_buf_is_valid(panel.buffer) and vim.api.nvim_win_is_valid(panel.window)
end

local function display_width(lines)
	local width = 1
	for _, line in ipairs(lines) do
		width = math.max(width, vim.fn.strdisplaywidth(line))
	end
	return width
end

local function truncate(line, width)
	if vim.fn.strdisplaywidth(line) <= width then
		return line
	end
	local result, index = "", 0
	while vim.fn.strdisplaywidth(result .. "…") < width do
		local character = vim.fn.strcharpart(line, index, 1)
		if character == "" then
			break
		end
		result, index = result .. character, index + 1
	end
	return result .. "…"
end

local function frame_lines(frame, message)
	local lines = vim.split(frame, "\n", { plain = true })
	lines[#lines] = lines[#lines] .. "  " .. message
	return lines
end

local function mode_index(animation)
	if #animation.frames == 1 then
		return 1
	end
	if animation.mode == "random" then
		local next_index = animation.random(#animation.frames)
		if next_index == animation.index then
			next_index = next_index % #animation.frames + 1
		end
		return next_index
	end
	if animation.mode == "reverse" then
		return animation.index == 1 and #animation.frames or animation.index - 1
	end
	if animation.mode == "swing" then
		local next_index = animation.index + animation.direction
		if next_index > #animation.frames then
			animation.direction, next_index = -1, #animation.frames - 1
		elseif next_index < 1 then
			animation.direction, next_index = 1, 2
		end
		return next_index
	end
	return animation.index % #animation.frames + 1
end

local function generated_frame(generator, random)
	local ranges = {
		random_dots = { base = 0x2800, count = 256 },
		mahjong = { base = 0x1F000, count = 44 },
		domino = { base = 0x1F030, count = 50 },
		vertical_domino = { base = 0x1F062, count = 50 },
	}
	local range = ranges[generator]
	if not range then
		fail("unknown generated spinner: " .. tostring(generator))
	end
	local offset = random(range.count)
	if type(offset) ~= "number" or offset % 1 ~= 0 or offset < 1 or offset > range.count then
		fail("random must return an integer within the requested range")
	end
	return vim.fn.nr2char(range.base + offset - 1)
end

function M.resolve(value)
	return resolve(value)
end

function M.configure(value, motion)
	settings = resolve(value)
	motion = motion or { enabled = true, reduced = false }
	if type(motion) ~= "table" or type(motion.enabled) ~= "boolean" or type(motion.reduced) ~= "boolean" then
		fail("motion settings require enabled and reduced")
	end
	settings.animate = motion.enabled and not motion.reduced
	return vim.deepcopy(settings)
end

function M.presets()
	return presets.names()
end

function M.preset(name)
	return presets.get(name)
end

function M.animation(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (vim.islist(opts) and next(opts) ~= nil) then
		fail("animation options must be an object")
	end
	for key in pairs(opts) do
		if key ~= "spinner" and key ~= "interval_ms" and key ~= "random" then
			fail("animation options contain unsupported field: " .. tostring(key))
		end
	end
	local preset = presets.get(opts.spinner or settings.spinner)
	if not preset then
		fail("spinner must name a bundled Rattles or Whirly preset")
	end
	local interval = opts.interval_ms == nil and settings.interval_ms or opts.interval_ms
	if type(interval) ~= "number" or interval % 1 ~= 0 or interval < 0 or (interval > 0 and interval < 16) then
		fail("interval_ms must be 0 or an integer of at least 16")
	end
	local random = opts.random or math.random
	if type(random) ~= "function" then
		fail("random must be a function")
	end
	local animation = {
		frames = preset.frames,
		interval_ms = interval == 0 and preset.interval_ms or interval,
		mode = preset.mode,
		index = preset.mode == "reverse" and #(preset.frames or {}) or 1,
		direction = 1,
		random = random,
	}
	if preset.generator then
		animation.generator = preset.generator
		animation.frame = generated_frame(animation.generator, random)
	end
	return animation
end

function M.advance(animation)
	if
		type(animation) ~= "table"
		or (not animation.generator and (type(animation.frames) ~= "table" or #animation.frames == 0))
	then
		fail("advance requires an animation")
	end
	if animation.generator then
		animation.frame = generated_frame(animation.generator, animation.random)
		return animation.frame
	end
	animation.index = mode_index(animation)
	return animation.frames[animation.index]
end

function M.frame(animation)
	if type(animation) ~= "table" then
		fail("frame requires an animation")
	end
	return animation.generator and animation.frame or animation.frames[animation.index]
end

local function render(panel)
	if not panel_valid(panel) then
		return false
	end
	local lines = frame_lines(M.frame(panel.animation), panel.message)
	local maximum = math.max(vim.o.columns - 4, 1)
	local width = math.min(display_width(lines), maximum)
	for index, line in ipairs(lines) do
		lines[index] = truncate(line, width)
	end
	vim.bo[panel.buffer].modifiable = true
	vim.api.nvim_buf_set_lines(panel.buffer, 0, -1, false, lines)
	vim.bo[panel.buffer].modifiable = false
	vim.api.nvim_win_set_config(panel.window, {
		relative = "editor",
		row = math.max(math.floor((vim.o.lines - #lines) / 2) + panel.offset, 0),
		col = math.max(math.floor((vim.o.columns - width - 2) / 2), 0),
		width = width,
		height = #lines,
	})
	return true
end

local function stop_timer(panel)
	local timer = panel.timer
	panel.timer = nil
	if timer and not timer:is_closing() then
		timer:stop()
		timer:close()
	end
end

local function schedule(panel)
	if panel.closed or not panel.animate then
		return
	end
	local timer = vim.uv.new_timer()
	panel.timer = timer
	timer:start(
		panel.animation.interval_ms,
		panel.animation.interval_ms,
		vim.schedule_wrap(function()
			if panel.closed or panel.timer ~= timer or not panel_valid(panel) then
				stop_timer(panel)
				return
			end
			M.advance(panel.animation)
			render(panel)
		end)
	)
end

function M.open(opts)
	opts = opts or {}
	if type(opts) ~= "table" or (vim.islist(opts) and next(opts) ~= nil) then
		fail("open options must be an object")
	end
	for key in pairs(opts) do
		if key ~= "message" and key ~= "spinner" and key ~= "interval_ms" and key ~= "force" then
			fail("open options contain unsupported field: " .. tostring(key))
		end
	end
	if type(opts.message) ~= "string" or vim.trim(opts.message) == "" then
		fail("message must be non-empty text")
	end
	if opts.force ~= nil and type(opts.force) ~= "boolean" then
		fail("force must be boolean")
	end
	if (not settings.enabled and not opts.force) or #vim.api.nvim_list_uis() == 0 then
		return { close = function() end, update = function() end }
	end
	sequence = sequence + 1
	local active_count = 0
	for _ in pairs(panels) do
		active_count = active_count + 1
	end
	local buffer = vim.api.nvim_create_buf(false, true)
	vim.bo[buffer].bufhidden, vim.bo[buffer].filetype = "wipe", "gator-loading"
	local window = vim.api.nvim_open_win(buffer, false, {
		relative = "editor",
		row = 0,
		col = 0,
		width = 1,
		height = 1,
		style = "minimal",
		focusable = false,
		border = "rounded",
		noautocmd = true,
	})
	local panel = {
		id = sequence,
		buffer = buffer,
		window = window,
		message = opts.message,
		animation = M.animation({ spinner = opts.spinner, interval_ms = opts.interval_ms }),
		animate = settings.animate,
		offset = active_count * 3,
		generation = 0,
		closed = false,
	}
	panels[panel.id] = panel
	render(panel)
	schedule(panel)
	local handle = {}
	function handle.update(message)
		if panel.closed then
			return false
		end
		if type(message) ~= "string" or vim.trim(message) == "" then
			fail("updated message must be non-empty text")
		end
		panel.message = message
		return render(panel)
	end
	function handle.close()
		if panel.closed then
			return false
		end
		panel.closed, panel.generation, panels[panel.id] = true, panel.generation + 1, nil
		stop_timer(panel)
		if vim.api.nvim_win_is_valid(panel.window) then
			vim.api.nvim_win_close(panel.window, true)
		end
		return true
	end
	return handle
end

function M.close()
	for _, panel in pairs(panels) do
		panel.closed, panel.generation = true, panel.generation + 1
		stop_timer(panel)
		if vim.api.nvim_win_is_valid(panel.window) then
			vim.api.nvim_win_close(panel.window, true)
		end
	end
	panels = {}
	return true
end

return M
