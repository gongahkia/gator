local files = {}

for _, root in ipairs({ "lua", "plugin", "tests" }) do
	vim.list_extend(files, vim.fn.glob(root .. "/**/*.lua", false, true))
end

table.sort(files)
assert(#files > 0, "Lua lint target found no files")

for _, path in ipairs(files) do
	local chunk, err = loadfile(path)
	if not chunk then
		vim.api.nvim_err_writeln(path .. ": " .. err)
		vim.cmd("cquit 1")
	end
end

vim.cmd("qa!")
