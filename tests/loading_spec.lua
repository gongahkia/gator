local root = vim.g.gator_test.root
local helpers = dofile(root .. "/tests/helpers.lua")
local commands = vim.api.nvim_get_commands({ builtin = false })

assert(not commands.Gator, "lazy module loading must not register commands")
assert(not commands.GatorHealth, "lazy module loading must not register commands")
assert(not commands.GatorExportDiagnostics, "lazy module loading must not register diagnostic commands")
assert(not commands.GatorBetaReadiness, "lazy module loading must not register beta readiness commands")
assert(not commands.GatorCaptureSelection, "lazy module loading must not register context commands")
assert(not commands.GatorPalette, "lazy module loading must not register palette commands")

local gator = require("gator").setup()
assert(gator._state, "lazy module loading must support setup")

local packpath = helpers.tempdir("packpath")
local package_dir = packpath .. "/pack/gator/start/gator"
assert(vim.fn.mkdir(vim.fn.fnamemodify(package_dir, ":h"), "p") == 1, "must create native package directory")
assert(vim.uv.fs_symlink(root, package_dir, { dir = true }), "must link native package fixture")

vim.o.packpath = packpath
vim.cmd("packadd gator")

commands = vim.api.nvim_get_commands({ builtin = false })
assert(commands.Gator, "native package loading must register :Gator")
assert(commands.GatorHealth, "native package loading must register :GatorHealth")
assert(commands.GatorExportDiagnostics, "native package loading must register diagnostic export")
assert(commands.GatorBetaReadiness, "native package loading must register beta readiness")
assert(commands.GatorCaptureSelection, "native package loading must register selection capture")
assert(commands.GatorPalette, "native package loading must register the command palette")
assert(require("gator")._state == gator._state, "plugin loading must preserve configured state")
