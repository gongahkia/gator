local ok, err = xpcall(function()
  dofile("tests/loading_spec.lua")
  require("gator")._test()
end, debug.traceback)

if not ok then
  vim.api.nvim_err_writeln(err)
  vim.cmd("cquit 1")
end

vim.cmd("qa!")
