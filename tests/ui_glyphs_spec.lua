local accessibility = require("gator.ui").accessibility
local config = require("gator.config")
local glyphs = require("gator.ui.glyphs")

assert(
	config.resolve({ ui = { icons = "nerd_font" } }).ui.icons == "nerd_font"
		and config.resolve({ ui = { icons = "ascii" } }).ui.icons == "ascii"
		and not pcall(config.resolve, { ui = { icons = "unsupported" } }),
	"UI glyph style must accept Unicode, Nerd Font, and ASCII modes only"
)

glyphs.configure("unicode")
local unicode = glyphs.decorate({ "Gator workspace", "State: ready", "Tasks: empty", "> Open tasks", "j/k navigate · q close · ? help" }, "gator")
assert(
	unicode[1] == "🐊 Gator workspace"
		and unicode[2] == "✅ State: ready"
		and unicode[3] == "📋 Tasks: empty"
		and unicode[4] == "> Open tasks 🐊"
		and unicode[5] == "⌨ j/k navigate · q close · ? help",
	"Unicode panels must use alligator and semantic glyphs"
)

glyphs.configure("nerd_font")
assert(glyphs.get("task") ~= "📋" and glyphs.decorate({ "Gator workspace" }, "gator")[1]:find("Gator workspace", 1, true), "Nerd Font mode must retain panel text")

glyphs.configure("ascii")
local ascii = glyphs.decorate({ "Gator workspace", "No matching tasks" }, "gator")
assert(ascii[1] == "[G] Gator workspace" and ascii[2] == "[-] No matching tasks", "ASCII mode must avoid non-ASCII glyphs")

local buffer = vim.api.nvim_create_buf(false, true)
accessibility.configure({ keymaps = {}, screen_reader = true, icons = "unicode" })
accessibility.render(buffer, { "Gator workspace", "State: ready" }, "gator")
assert(
	vim.deep_equal(vim.api.nvim_buf_get_lines(buffer, 0, -1, false), { "🐊 Gator workspace", "✅ State: ready" }),
	"accessibility rendering must preserve configured visual glyphs"
)

glyphs.configure("unicode")
