local health = require("gator.health")
local original_system = vim.system
local calls, timer = 0, nil
vim.system = function(_, _, callback)
	calls = calls + 1
	timer = vim.uv.new_timer()
	timer:start(1, 0, function()
		timer:stop()
		timer:close()
		callback({ code = 0, stdout = "true\n" })
	end)
	return { kill = function() end }
end
local records = health.readiness({
	executable = function(name)
		return name == "git"
	end,
})
vim.system = original_system
assert(calls == 1, "health readiness must probe the Git workspace through the yielding command runner")
assert(
	vim.tbl_contains(
		vim.tbl_map(function(record)
			return record.component
		end, records),
		"workspace"
	),
	"health readiness must retain the workspace result after an asynchronous command callback"
)
