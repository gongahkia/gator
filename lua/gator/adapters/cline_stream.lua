local M = {}

local function fail(message)
	error("Gator Cline stream: " .. message, 3)
end

local function event(value, index)
	if type(value) ~= "table" or (value.type ~= "ask" and value.type ~= "say") then
		fail("record " .. index .. " must declare documented ask or say type")
	end
	if type(value.text) ~= "string" then
		fail("record " .. index .. " text must be a string")
	end
	if type(value.ts) ~= "number" or value.ts < 0 or value.ts % 1 ~= 0 then
		fail("record " .. index .. " ts must be a non-negative integer")
	end
	local subtype = value[value.type]
	if type(subtype) ~= "string" or subtype == "" then
		fail("record " .. index .. " must declare a non-empty " .. value.type .. " subtype")
	end
	if value.reasoning ~= nil and type(value.reasoning) ~= "string" then
		fail("record " .. index .. " reasoning must be a string")
	end
	if value.partial ~= nil and type(value.partial) ~= "boolean" then
		fail("record " .. index .. " partial must be a boolean")
	end
	return {
		type = value.type,
		text = value.text,
		ts = value.ts,
		subtype = subtype,
		reasoning = value.reasoning,
		partial = value.partial,
	}
end

function M.parse(output)
	if type(output) ~= "string" or output == "" then
		fail("stream output must be a non-empty JSONL string")
	end
	local events = {}
	for index, line in ipairs(vim.split(output, "\n", { plain = true, trimempty = true })) do
		local ok, value = pcall(vim.json.decode, line)
		if not ok then
			fail("record " .. index .. " is not JSON")
		end
		events[index] = event(value, index)
	end
	if #events == 0 then
		fail("stream output contains no JSON records")
	end
	return events
end

return M
