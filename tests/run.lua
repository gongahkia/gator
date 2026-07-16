local root = vim.g.gator_test.root
local helpers = dofile(root .. "/tests/helpers.lua")
local specs = vim.fn.glob(root .. "/tests/*_spec.lua", false, true)

table.sort(specs)

local ok, err = xpcall(function()
  for _, spec in ipairs(specs) do
    dofile(spec)
  end
  require("gator")._test()
end, debug.traceback)

helpers.cleanup()

if not ok then
  vim.api.nvim_err_writeln(err)
  vim.cmd("cquit 1")
end

vim.cmd("qa!")
