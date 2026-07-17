local M = {}

local function fail(message)
	error("Gator Droid stream: " .. message, 3)
end

local function integer(value, name)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail(name .. " must be a non-negative integer")
	end
	return value
end

local function id(value)
	if type(value) ~= "string" or value == "" then
		fail("result session_id must be a non-empty opaque id")
	end
	return value
end

function M.parse(output)
	if type(output) ~= "string" or vim.trim(output) == "" then
		fail("output must be a non-empty JSON result")
	end
	local ok, result = pcall(vim.json.decode, vim.trim(output))
	if not ok or type(result) ~= "table" or vim.islist(result) then
		fail("output is not a JSON object")
	end
	if result.type ~= "result" then
		fail("output must declare a result record")
	end
	if
		result.subtype ~= "success"
		and result.subtype ~= "error_during_execution"
		and result.subtype ~= "error_structured_output"
	then
		fail("result subtype is unsupported")
	end
	if type(result.is_error) ~= "boolean" or type(result.result) ~= "string" then
		fail("result must declare is_error and text")
	end
	if (result.subtype == "success") ~= (result.is_error == false) then
		fail("result subtype and error state disagree")
	end
	return {
		id = id(result.session_id),
		text = result.result,
		subtype = result.subtype,
		is_error = result.is_error,
		duration_ms = integer(result.duration_ms, "result duration_ms"),
		num_turns = integer(result.num_turns, "result num_turns"),
	}
end

function M.parse_jsonrpc(output)
	if type(output) ~= "string" or output == "" then
		fail("JSON-RPC output must be a non-empty JSONL string")
	end
	local records = {}
	for index, line in ipairs(vim.split(output, "\n", { plain = true, trimempty = true })) do
		local ok, record = pcall(vim.json.decode, line)
		if not ok or type(record) ~= "table" or vim.islist(record) or record.jsonrpc ~= "2.0" then
			fail("JSON-RPC record " .. index .. " is invalid")
		end
		if record.method ~= nil then
			if
				type(record.method) ~= "string"
				or record.method == ""
				or record.result ~= nil
				or record.error ~= nil
			then
				fail("JSON-RPC request " .. index .. " is invalid")
			end
			if record.id == nil and record.method == "droid.session_notification" then
				if
					type(record.params) ~= "table"
					or vim.islist(record.params)
					or type(record.params.type) ~= "string"
					or record.params.type == ""
				then
					fail("Droid session notification " .. index .. " is invalid")
				end
			end
			records[index] = {
				kind = record.id == nil and "notification" or "request",
				method = record.method,
				id = record.id,
				params = vim.deepcopy(record.params),
			}
		elseif
			record.id == nil
			or (record.result == nil and record.error == nil)
			or (record.result ~= nil and record.error ~= nil)
		then
			fail("JSON-RPC response " .. index .. " is invalid")
		else
			records[index] = {
				kind = "response",
				id = record.id,
				result = vim.deepcopy(record.result),
				error = vim.deepcopy(record.error),
			}
		end
	end
	if #records == 0 then
		fail("JSON-RPC output contains no records")
	end
	return records
end

return M
