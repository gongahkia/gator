local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local diagnostic = require("gator").module("context").diagnostic
local path = helpers.tempdir("context-diagnostic") .. "/current.txt"
helpers.write(path, "first\nsecond\nthird")
local buffer = vim.api.nvim_create_buf(true, false)
vim.api.nvim_buf_set_name(buffer, path)
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "first", "second", "third" })
local namespace = vim.api.nvim_create_namespace("gator-context-diagnostic")
vim.diagnostic.set(namespace, buffer, {
	{
		lnum = 1,
		col = 2,
		end_lnum = 1,
		end_col = 5,
		severity = vim.diagnostic.severity.ERROR,
		source = "test-lsp",
		code = "E2",
		message = "second is invalid",
	},
	{
		lnum = 0,
		col = 0,
		severity = vim.diagnostic.severity.WARN,
		message = "first needs review",
	},
})

local records = diagnostic.capture({ buffer = buffer })
assert(#records == 2, "diagnostic capture must preserve current diagnostics")
assert(
	records[1].source == "gator-context-diagnostic"
		and records[1].severity == "warning"
		and records[1].location.first_line == 1
		and records[2].source == "test-lsp"
		and records[2].severity == "error"
		and records[2].location.first_column == 3,
	"diagnostic capture must preserve source, severity, and one-based locations"
)
assert(
	records[2].entry.id == "diagnostic-" .. records[2].fingerprint
		and records[2].entry.provenance.source == "test-lsp"
		and records[2].entry.ref:find("#sha256=" .. records[2].fingerprint, 1, true),
	"diagnostic entries must be content-addressed with provenance"
)
assert(not diagnostic.is_stale(records[1]), "unchanged diagnostics must not be stale")
vim.diagnostic.reset(namespace, buffer)
assert(diagnostic.is_stale(records[1]), "removed diagnostics must be stale")
vim.diagnostic.set(namespace, buffer, {
	{
		lnum = 0,
		col = 0,
		severity = vim.diagnostic.severity.INFO,
		message = "first changed",
	},
})
local current = diagnostic.capture({ buffer = buffer })[1]
vim.api.nvim_buf_set_lines(buffer, 0, 1, false, { "changed" })
assert(diagnostic.is_stale(current), "buffer revision changes must stale diagnostic records")
local unnamed = vim.api.nvim_create_buf(true, false)
assert(not pcall(diagnostic.capture, { buffer = unnamed }), "unnamed diagnostic buffers must fail explicitly")
assert(
	not pcall(diagnostic.capture, { buffer = buffer, token = "secret" }),
	"diagnostic capture must reject unsupported fields"
)
