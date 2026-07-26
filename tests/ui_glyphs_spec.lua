local accessibility = require("gator.ui").accessibility
local config = require("gator.config")
local glyphs = require("gator.ui.glyphs")

assert(
	config.resolve({ ui = { icons = "nerd_font" } }).ui.icons == "nerd_font"
		and config.resolve({ ui = { icons = "ascii" } }).ui.icons == "ascii"
		and config.resolve({ ui = { icons = "none" } }).ui.icons == "none"
		and not pcall(config.resolve, { ui = { icons = "unsupported" } }),
	"UI glyph style must accept Unicode, Nerd Font, ASCII, and disabled modes only"
)

glyphs.configure("unicode")
local unicode = glyphs.decorate(
	{ "Gator runs", "State: ready", "No Gator-managed runs", "> Open runs", "j/k navigate · q close · ? help" },
	"gator"
)
assert(
	unicode[1] == "🐊 Gator runs"
		and unicode[2] == "✅ State: ready"
		and unicode[3] == "∅ No Gator-managed runs"
		and unicode[4] == "> Open runs"
		and unicode[5] == "⌨ j/k navigate · q close · ? help",
	"Unicode panels must use alligator and semantic glyphs"
)

glyphs.configure("nerd_font")
assert(
	glyphs.get("brand") ~= "🐊" and glyphs.decorate({ "Gator runs" }, "gator")[1]:find("Gator runs", 1, true),
	"Nerd Font mode must retain panel text"
)

glyphs.configure("ascii")
local ascii = glyphs.decorate({ "Gator runs", "No matching runs" }, "gator")
assert(ascii[1] == "[G] Gator runs" and ascii[2] == "[-] No matching runs", "ASCII mode must avoid non-ASCII glyphs")

glyphs.configure("none")
assert(
	vim.deep_equal(glyphs.decorate({ "Gator runs", "State: ready", "No Gator-managed runs" }, "gator"), {
		"Gator runs",
		"State: ready",
		"No Gator-managed runs",
	}),
	"disabled mode must retain only panel text"
)

local buffer = vim.api.nvim_create_buf(false, true)
accessibility.configure({ keymaps = {}, screen_reader = true, icons = "unicode" })
accessibility.render(buffer, { "Gator runs", "State: ready" }, "gator")
assert(
	vim.deep_equal(vim.api.nvim_buf_get_lines(buffer, 0, -1, false), { "🐊 Gator runs", "✅ State: ready" }),
	"accessibility rendering must preserve configured visual glyphs"
)

glyphs.configure("unicode")
