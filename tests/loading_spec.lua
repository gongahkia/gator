local loading = require("gator.ui.loading")

local names = loading.presets()
assert(#names == 169, "loading presets must vendor every Rattles and Whirly animation")
local sources = { rattles = 0, whirly = 0 }
for _, name in ipairs(names) do
	sources[loading.preset(name).source] = sources[loading.preset(name).source] + 1
end
assert(
	sources.rattles == 56 and sources.whirly == 113,
	"loading preset catalog must retain every upstream Rattles and Whirly entry"
)
assert(
	loading.preset("rattles.braille.dots").interval_ms == 80
		and loading.preset("rattles.braille.dots").source == "rattles"
		and loading.preset("whirly.hanoi").mode == "swing"
		and loading.preset("whirly.cli.dots").source == "whirly",
	"loading presets must retain upstream provenance, timing, and modes"
)

loading.configure({ enabled = true, spinner = "whirly.hanoi", interval_ms = 0 }, { enabled = true, reduced = false })
local swing = loading.animation()
assert(
	swing.frames[swing.index] == "𝍥"
		and loading.advance(swing) == "𝍦"
		and loading.advance(swing) == "𝍧"
		and loading.advance(swing) == "𝍨"
		and loading.advance(swing) == "𝍧",
	"Whirly swing spinners must reverse at the final frame"
)

loading.configure({ enabled = true, spinner = "whirly.dice", interval_ms = 16 }, { enabled = true, reduced = false })
local random = loading.animation({
	random = function()
		return 1
	end,
})
assert(loading.advance(random) == "⚁", "random spinners must avoid repeating the previous frame")
loading.configure({ enabled = true, spinner = "whirly.mahjong", interval_ms = 0 }, { enabled = true, reduced = false })
local generated = loading.animation({
	random = function(count)
		assert(count == 44, "generated spinners must preserve their upstream codepoint range")
		return 1
	end,
})
assert(loading.frame(generated) == "🀀", "Whirly procedural spinners must remain selectable")
assert(not pcall(loading.configure, { spinner = "missing" }), "unknown loading spinners must fail configuration")
