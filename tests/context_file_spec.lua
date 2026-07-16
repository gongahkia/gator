local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local file = require("gator").module("context").file
local path = helpers.tempdir("context-file") .. "/current.txt"
helpers.write(path, "alpha\nbeta")
local buffer = vim.api.nvim_create_buf(true, false)
vim.api.nvim_buf_set_name(buffer, path)
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "alpha", "beta" })
vim.bo[buffer].endofline = false
vim.bo[buffer].filetype = "gator-test"

local first = file.capture({ buffer = buffer })
local second = file.capture({ buffer = buffer })
assert(first.path == vim.uv.fs_realpath(path), "file context must canonicalize the file path")
assert(first.digest == vim.fn.sha256("alpha\nbeta"), "file context must content-address buffer text")
assert(
	first.entry.id == "file-" .. first.digest
		and first.entry.ref == "file://" .. first.path .. "#sha256=" .. first.digest,
	"file context references must be stable and content-addressed"
)
assert(
	first.entry.provenance.source == "buffer" and first.entry.provenance.ref == first.path,
	"file context must retain buffer provenance"
)
assert(
	first.language == "gator-test" and first.revision == second.revision and first.entry.ref == second.entry.ref,
	"unchanged files must retain stable context metadata"
)

vim.api.nvim_buf_set_lines(buffer, 1, 2, false, { "changed" })
local changed = file.capture({ buffer = buffer })
assert(
	changed.digest ~= first.digest and changed.entry.ref ~= first.entry.ref and changed.revision > first.revision,
	"changed file content must receive a new reference and revision"
)
local unnamed = vim.api.nvim_create_buf(true, false)
assert(not pcall(file.capture, { buffer = unnamed }), "unnamed buffers must fail explicitly")
assert(not pcall(file.capture, { buffer = buffer, token = "secret" }), "file capture must reject unsupported fields")
