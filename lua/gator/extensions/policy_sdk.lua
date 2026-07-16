local overlay = require("gator.policy.overlay")
local M = { api_version = 1 }
local modes = { read_only = 0, plan = 1, default = 2 }

local function fail(message)
	error("Gator policy SDK: " .. message, 3)
end

local function identifier(value, name)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function narrow(baseline, rules)
	if type(rules) ~= "table" then
		fail("decision rules must be a table")
	end
	for key, value in pairs(rules) do
		if key:match("_allowed$") then
			if type(value) ~= "boolean" or (value and baseline.rules[key] ~= true) then
				fail("decision " .. key .. " would broaden the baseline")
			end
		elseif key == "mode" then
			local base = baseline.rules.mode or "default"
			if type(value) ~= "string" or modes[value] == nil or modes[value] > modes[base] then
				fail("decision mode would broaden the baseline")
			end
		elseif not vim.deep_equal(value, baseline.rules[key]) then
			fail("decision rule is not an approved narrowing: " .. key)
		end
	end
	return vim.deepcopy(rules)
end

function M.define(attrs)
	if type(attrs) ~= "table" or type(attrs.evaluate) ~= "function" then
		fail("define requires an evaluator")
	end
	for key in pairs(attrs) do
		if key ~= "name" and key ~= "evaluate" then
			fail("evaluator contains unsupported field: " .. tostring(key))
		end
	end
	local name = identifier(attrs.name, "evaluator name")
	return {
		name = name,
		evaluate = function(request)
			if
				type(request) ~= "table"
				or type(request.action) ~= "string"
				or request.action == ""
				or not overlay.is(request.baseline)
			then
				fail("evaluate requires action and baseline policy")
			end
			local ok, decision =
				pcall(attrs.evaluate, { action = request.action, baseline = overlay.to_record(request.baseline) })
			if
				not ok
				or type(decision) ~= "table"
				or type(decision.allowed) ~= "boolean"
				or type(decision.reason) ~= "string"
				or decision.reason == ""
			then
				fail("evaluator must return allowed and reason")
			end
			for key in pairs(decision) do
				if key ~= "allowed" and key ~= "reason" and key ~= "rules" then
					fail("evaluator decision contains unsupported field: " .. tostring(key))
				end
			end
			return {
				evaluator = name,
				allowed = decision.allowed,
				reason = decision.reason,
				rules = narrow(request.baseline, decision.rules or {}),
			}
		end,
	}
end

return M
