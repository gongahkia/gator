local source = debug.getinfo(1, "S").source:sub(2)
local root = vim.fn.fnamemodify(source, ":h:h")
local tempdir = vim.fn.tempname()

assert(vim.fn.mkdir(tempdir, "p") == 1, "must create isolated test state")
vim.env.XDG_CACHE_HOME = tempdir .. "/cache"
vim.env.XDG_CONFIG_HOME = tempdir .. "/config"
vim.env.XDG_DATA_HOME = tempdir .. "/data"
vim.env.XDG_STATE_HOME = tempdir .. "/state"
vim.opt.runtimepath = { root, vim.env.VIMRUNTIME }
vim.opt.packpath = ""
vim.g.gator_test = { root = root, tempdir = tempdir }
