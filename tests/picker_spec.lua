local picker = require("gator.ui").picker
local selected
local cancelled = false
local window = picker.open({
	title = "Gator actions",
	items = { { id = "open", label = "Open dashboard" }, { id = "review", label = "Review changes" } },
	on_select = function(item)
		selected = item
	end,
	on_cancel = function()
		cancelled = true
	end,
})

assert(vim.api.nvim_win_is_valid(window), "native picker opening must create a window")
assert(#picker.filter("review") == 1, "native picker must filter without optional dependencies")
assert(picker.select(1).id == "review", "native picker must select visible items")
assert(
	picker.confirm().id == "review" and selected.id == "review",
	"native picker confirmation must route the selected item"
)
picker.open({
	title = "Gator actions",
	items = {},
	on_select = function() end,
	on_cancel = function()
		cancelled = true
	end,
})
assert(picker.cancel() and cancelled, "native picker cancellation must run its callback and close")
local ok = pcall(picker.open, {
	title = "invalid",
	items = { { id = "one", label = "one", extra = true } },
	on_select = function() end,
})
assert(not ok, "native picker items must reject unsupported fields")
