local motion = require("gator.ui").motion
local pending = {}
local frames = {}
local spinner = motion.spinner({
	settings = { enabled = true, interval_ms = 16, reduced = false },
	frames = { "one", "two" },
	defer = function(callback)
		pending[1] = callback
	end,
})
spinner.start(function(frame)
	table.insert(frames, frame)
end)
assert(frames[1] == "one", "motion spinner must render immediately")
pending[1]()
assert(frames[2] == "two", "motion spinner must advance without blocking")
spinner.stop()
pending[1]()
assert(#frames == 2, "stopped motion spinners must cancel scheduled frames")

local steps = {}
pending = {}
motion.transition({
	from = 1,
	to = 4,
	steps = 3,
	settings = { enabled = true, interval_ms = 16, reduced = false },
	defer = function(callback)
		pending[1] = callback
	end,
	render = function(value, done)
		table.insert(steps, { value = value, done = done })
	end,
})
pending[1]()
pending[1]()
pending[1]()
assert(steps[1].value == 1 and steps[4].value == 4 and steps[4].done, "motion transitions must reach their target")

local reduced = {}
motion.transition({
	from = 1,
	to = 4,
	steps = 3,
	settings = { enabled = true, interval_ms = 16, reduced = true },
	render = function(value, done)
		table.insert(reduced, { value = value, done = done })
	end,
})
assert(#reduced == 1 and reduced[1].value == 4 and reduced[1].done, "reduced motion must complete without animation")

assert(
	require("gator.config").resolve({ ui = { motion = { enabled = false, interval_ms = 16, reduced = false } } }).ui.motion.enabled
		== false,
	"UI motion settings must remain configurable"
)
