local fixtures = require("gator.adapters.fixtures")
local M = {
	api_version = 1,
	fixtures = fixtures,
	kinds = { ["local"] = true, cloud = true, lexical = true, agent_managed = true },
}

local function fail(message)
	error("Gator retrieval SDK: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function result(value, provider)
	if type(value) ~= "table" or not vim.islist(value) then
		fail("retrieve must return a result list")
	end
	local records = {}
	for index, entry in ipairs(value) do
		if type(entry) ~= "table" then
			fail("result " .. index .. " must be an object")
		end
		for key in pairs(entry) do
			if key ~= "ref" and key ~= "content" and key ~= "score" then
				fail("result " .. index .. " contains unsupported field: " .. tostring(key))
			end
		end
		if type(entry.ref) ~= "string" or entry.ref == "" then
			fail("result " .. index .. " ref must be a non-empty string")
		end
		if entry.content ~= nil and (type(entry.content) ~= "string" or entry.content == "") then
			fail("result " .. index .. " content must be a non-empty string")
		end
		if entry.score ~= nil and (type(entry.score) ~= "number" or entry.score < 0) then
			fail("result " .. index .. " score must be non-negative")
		end
		records[index] = { provider = provider, ref = entry.ref, content = entry.content, score = entry.score }
	end
	return records
end

function M.define(attrs)
	if type(attrs) ~= "table" then
		fail("define requires provider attributes")
	end
	for key in pairs(attrs) do
		if key ~= "name" and key ~= "kind" and key ~= "retrieve" then
			fail("provider contains unsupported field: " .. tostring(key))
		end
	end
	local name = identifier(attrs.name, "provider name")
	if type(attrs.kind) ~= "string" or not M.kinds[attrs.kind] then
		fail("provider kind is unsupported")
	end
	if type(attrs.retrieve) ~= "function" then
		fail("provider retrieve must be a function")
	end
	return {
		name = name,
		kind = attrs.kind,
		retrieve = function(request)
			if type(request) ~= "table" or type(request.query) ~= "string" or request.query == "" then
				fail("retrieve requires a non-empty query")
			end
			for key in pairs(request) do
				if key ~= "query" and key ~= "cwd" and key ~= "privacy_consent" then
					fail("retrieve request contains unsupported field: " .. tostring(key))
				end
			end
			if attrs.kind == "cloud" and request.privacy_consent ~= true then
				fail("cloud retrieval requires explicit privacy consent")
			end
			local ok, values = pcall(attrs.retrieve, vim.deepcopy(request))
			if not ok then
				fail("provider retrieval failed")
			end
			return result(values, name)
		end,
	}
end

return M
