local palette = require("gator.ui").palette
local invoked = {}

assert(palette.register({
	kind = "action",
	name = "open-dashboard",
	execute = function()
		table.insert(invoked, "action")
	end,
}) == "action:open-dashboard", "palette registration must expose action identifiers")
palette.register({
	kind = "adapter",
	name = "probe-codex",
	execute = function()
		table.insert(invoked, "adapter")
	end,
})
palette.register({
	kind = "task",
	name = "start-task",
	execute = function()
		table.insert(invoked, "task")
	end,
})
palette.register({
	kind = "provider",
	name = "resume-session",
	execute = function()
		table.insert(invoked, "provider")
	end,
})

assert(
	vim.deep_equal(palette.complete("adapter:"), { "adapter:probe-codex" }),
	"palette completion must filter registered adapter commands"
)
vim.cmd("GatorPalette task:start-task")
assert(invoked[1] == "task", "native palette command must route task commands")
vim.cmd("GatorPalette")
require("gator.ui").picker.filter("task:start-task")
require("gator.ui").picker.confirm()
assert(invoked[2] == "task", "empty palette command must open a selectable command palette")
assert(#palette.list() == 4, "palette must list every supported command kind")
assert(palette.unregister("provider:resume-session"), "palette entries must be removable")

local ok = pcall(palette.register, { kind = "provider", name = "bad", execute = true })
assert(not ok, "palette entries without executable callbacks must fail explicitly")
