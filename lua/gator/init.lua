local config = require("gator.config")
local state = require("gator.state")
local ui = require("gator.ui")

local M = {
  _state = nil,
  modules = {
    core = "gator.core",
    ui = "gator.ui",
    adapters = "gator.adapters",
    context = "gator.context",
    workspace = "gator.workspace",
    review = "gator.review",
    policy = "gator.policy",
  },
}

function M.module(name)
  local path = M.modules[name]
  if not path then error("unknown Gator module: " .. tostring(name)) end
  return require(path)
end

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
  for name in pairs(M.modules) do
    local module = M.module(name)
    assert(type(module) == "table", name .. " module must return a table")
    assert(module.name == name, name .. " module must identify itself")
    assert(module.api_version == 1, name .. " module must declare API version 1")
  end
  local ok = pcall(M.module, "missing")
  assert(not ok, "unknown modules must fail explicitly")
end

return M
