local config = require("gator.config")
local state = require("gator.state")
local ui = require("gator.ui")

local M = { _state = nil }

function M.setup(opts)
  M._state = state.new(config.resolve(opts))
  return M
end

function M.open()
  if not M._state then M.setup() end
  ui.open(M._state)
end

function M.health()
  local lines = {
    "Gator health",
    "Neovim: " .. vim.version().major .. "." .. vim.version().minor,
    "Git: " .. (vim.fn.executable("git") == 1 and "available" or "missing"),
    "Rust indexer: not configured",
  }
  vim.notify(table.concat(lines, "\n"))
end

function M._test()
  assert(M._state, "gator setup must initialize state")
  assert(M._state.config.context.mode == "manual", "manual context must be the default")
end

return M
