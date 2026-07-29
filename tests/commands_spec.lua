local commands = require("gator.commands")

commands.register()
local settings = { ask_selection = { keymap = "<leader>gA" }, edit_selection = { keymap = "<leader>gE" } }
assert(commands.configure(settings).ask, "an unclaimed Visual mapping must install")
local default = vim.fn.maparg("<leader>gA", "x", false, true)
assert(
	default.rhs == "<Plug>(gator-ask-selection)" and default.desc == "Gator ask about selected text",
	"default Visual mapping must invoke the stable Gator plug mapping"
)
assert(
	vim.fn.maparg("<Plug>(gator-ask-selection)", "x", false, true).rhs == ":<C-U>'<,'>GatorAsk<CR>",
	"selection asks must expose a stable remapping target"
)
local edit = vim.fn.maparg("<leader>gE", "x", false, true)
assert(
	edit.rhs == "<Plug>(gator-edit-selection)"
		and vim.fn.maparg("<Plug>(gator-edit-selection)", "x", false, true).rhs == ":<C-U>'<,'>GatorEdit<CR>",
	"selection edits must expose an unclaimed default and stable remapping target"
)
assert(
	not commands.configure({ ask_selection = { keymap = false }, edit_selection = { keymap = false } }).ask,
	"disabled selection mappings must not install a default"
)
assert(next(vim.fn.maparg("<leader>gA", "x", false, true)) == nil, "disabled selection mapping must remove Gator's map")

vim.keymap.set("x", "<leader>gA", "<Nop>", { desc = "user selection mapping" })
assert(
	not commands.configure({ ask_selection = { keymap = "<leader>gA" }, edit_selection = { keymap = false } }).ask,
	"an existing Visual mapping must be left untouched"
)
local claimed = vim.fn.maparg("<leader>gA", "x", false, true)
assert(claimed.rhs == "<Nop>" and claimed.desc == "user selection mapping", "Gator must not replace user mappings")
vim.keymap.del("x", "<leader>gA")
