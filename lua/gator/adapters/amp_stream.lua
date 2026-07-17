local M = {}

local function fail(message)
	error("Gator Amp stream: " .. message, 3)
end

local function session_id(value)
	if type(value) ~= "string" or value == "" then
		fail("event session_id must be a non-empty opaque id")
	end
	return value
end

function M.parse(output)
	if type(output) ~= "string" or output == "" then
		fail("stream output must be a non-empty JSONL string")
	end
	local events = {}
	local id
	local terminal
	for index, line in ipairs(vim.split(output, "\n", { plain = true, trimempty = true })) do
		local ok, event = pcall(vim.json.decode, line)
		if not ok or type(event) ~= "table" or vim.islist(event) or type(event.type) ~= "string" then
			fail("stream record " .. index .. " is invalid")
		end
		if terminal then
			fail("stream contains an event after its result")
		end
		local current = session_id(event.session_id)
		if id and current ~= id then
			fail("stream session ids must remain stable")
		end
		id = current
		if index == 1 and (event.type ~= "system" or event.subtype ~= "init") then
			fail("stream must begin with a system init event")
		end
		if event.type == "result" then
			if event.subtype ~= "success" or event.is_error ~= false or type(event.result) ~= "string" then
				fail("stream result is not a successful completion")
			end
			terminal = event
		end
		events[index] = vim.deepcopy(event)
	end
	if not terminal then
		fail("stream ended before a successful result")
	end
	return { id = id, result = terminal.result, events = events }
end

return M
