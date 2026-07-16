local pack = require("gator.context.pack")
local M = {}
local files = { "AGENTS.md", "CLAUDE.md", "GEMINI.md", ".github/copilot-instructions.md", ".gator/policy.json" }

local function fail(message)
	error("Gator instruction context: " .. message, 3)
end

function M.discover(opts)
	if type(opts) ~= "table" or type(opts.cwd) ~= "string" or opts.cwd == "" then
		fail("discover requires a cwd")
	end
	for key in pairs(opts) do
		if key ~= "cwd" then
			fail("options contain unsupported field: " .. tostring(key))
		end
	end
	local root = vim.uv.fs_realpath(opts.cwd)
	if not root or vim.fn.isdirectory(root) ~= 1 then
		fail("cwd must be an existing directory")
	end
	local result = {}
	for _, relative in ipairs(files) do
		local path = root .. "/" .. relative
		if vim.fn.filereadable(path) == 1 then
			local content = table.concat(vim.fn.readfile(path), "\n")
			local digest = vim.fn.sha256(content)
			table.insert(
				result,
				pack.entry({
					id = "instruction-" .. digest,
					kind = "file",
					ref = "file://" .. path .. "#sha256=" .. digest,
					provenance = { source = "project-instruction", ref = relative },
					trust = "provenance",
					token_estimate = {
						status = "unavailable",
						reason = "instruction content has not been provider-counted",
					},
					transfer = { eligible = false, reason = "project instructions require explicit trust confirmation" },
				})
			)
		end
	end
	return result
end

return M
