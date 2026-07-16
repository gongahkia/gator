local M = { version = 1, methods = { health = true, index = true, cancel = true } }

local function fail(message)
	error("Gator indexer protocol: " .. message, 3)
end

function M.request(id, method, params)
	if type(id) ~= "string" or id == "" or not M.methods[method] or (params ~= nil and type(params) ~= "table") then
		fail("request requires id, supported method, and optional params")
	end
	return { version = M.version, id = id, method = method, params = params or {} }
end

function M.response(value)
	if type(value) ~= "table" or value.version ~= M.version or type(value.ok) ~= "boolean" then
		fail("response must declare this protocol version and boolean ok")
	end
	if value.ok then
		if type(value.id) ~= "string" or type(value.result) ~= "table" then
			fail("successful response requires id and result")
		end
	else
		if type(value.error) ~= "string" or value.error == "" then
			fail("failed response requires error")
		end
	end
	return value
end

return M
