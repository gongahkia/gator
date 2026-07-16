local lexical = require("gator").module("context").lexical
local calls, queue, result, failure = {}, {}, nil, nil
local root = vim.g.gator_test.root
local function record(path, line, text)
	return vim.json.encode({
		type = "match",
		data = { path = { text = path }, lines = { text = text }, line_number = line },
	})
end
lexical.search_async({
	cwd = root,
	query = "needle",
	batch_size = 1,
	spawn = function(argv, cwd, callback)
		table.insert(calls, { argv = argv, cwd = cwd, callback = callback })
		return { pid = #calls }
	end,
	schedule = function(callback)
		table.insert(queue, callback)
	end,
	signal = function()
		return { available = false, reason = "fixture" }
	end,
	on_result = function(value)
		result = value
	end,
	on_error = function(message)
		failure = message
	end,
})
assert(#calls == 1 and not result, "asynchronous scans must defer Git and ripgrep work")
calls[1].callback({ code = 0, stdout = root .. "\n" })
assert(#calls == 2 and calls[2].argv[1] == "rg", "asynchronous scans must launch ripgrep only after Git resolves")
calls[2].callback({
	code = 0,
	stdout = record("one.lua", 1, "needle one") .. "\n" .. record("two.lua", 2, "needle two") .. "\n",
})
assert(not result and #queue == 1, "large scan output must parse in scheduled batches")
while #queue > 0 do
	table.remove(queue, 1)()
end
assert(
	not failure and #result == 2 and result[1].path == root .. "/one.lua",
	"asynchronous scans must return ordered matches"
)
