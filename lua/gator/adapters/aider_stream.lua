local M = {}

local function fail(message)
	error("Gator Aider stream: " .. message, 3)
end

function M.parse(output)
	if type(output) ~= "string" or output == "" then
		fail("stream output must be a non-empty terminal string")
	end
	local text = output:gsub("\27%[[%d;?]*[ -/]*[@-~]", "")
	if text == "" then
		fail("stream output contains no visible text")
	end
	return { text = text }
end

return M
