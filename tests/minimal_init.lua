local source = debug.getinfo(1, "S").source:sub(2)
local root = vim.fn.fnamemodify(source, ":h:h")
local tempdir = vim.fn.tempname()

assert(vim.fn.mkdir(tempdir, "p") == 1, "must create isolated test state")
vim.env.XDG_CACHE_HOME = tempdir .. "/cache"
vim.env.XDG_CONFIG_HOME = tempdir .. "/config"
vim.env.XDG_DATA_HOME = tempdir .. "/data"
vim.env.XDG_STATE_HOME = tempdir .. "/state"
for _, path in ipairs({ vim.env.XDG_CACHE_HOME, vim.env.XDG_CONFIG_HOME, vim.env.XDG_DATA_HOME, vim.env.XDG_STATE_HOME }) do
	assert(vim.fn.mkdir(path, "p") == 1, "must create isolated XDG state")
end
vim.opt.swapfile = false
vim.opt.directory = tempdir .. "/swap//"
assert(vim.fn.mkdir(tempdir .. "/swap", "p") == 1, "must create isolated swap state")
vim.opt.runtimepath = { root, vim.env.VIMRUNTIME }
vim.opt.packpath = ""
vim.g.gator_test = { root = root, tempdir = tempdir }
