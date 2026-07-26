local accessibility = require("gator.ui.accessibility")

local buffer = vim.api.nvim_create_buf(false, true)
accessibility.render(buffer, {
	"Gator workspace",
	"State: ready · provider passed",
	"Providers:",
	"> Open run graph",
	"<CR> confirm · j/k navigate · q close · ? help",
	"Review: unavailable · no evidence",
	"State: failed · request rejected",
}, "gator")

local groups = {}
for _, mark in ipairs(vim.api.nvim_buf_get_extmarks(buffer, -1, 0, -1, { details = true })) do
	groups[mark[4].hl_group] = true
end

for _, group in ipairs({
	"GatorTitle",
	"GatorLabel",
	"GatorSection",
	"GatorAction",
	"GatorKeymap",
	"GatorSuccess",
	"GatorWarning",
	"GatorError",
}) do
	assert(groups[group], "Gator text panes must expose " .. group)
end
