local core_run = require("gator.core.run")
local overlay = require("gator.policy.overlay")
local M = {}
local modes = { read_only = 0, plan = 1, default = 2 }

local function fail(message)
	error("Gator run policy: " .. message, 3)
end

local function timestamp(value)
	if type(value) ~= "number" or value < 0 or value % 1 ~= 0 then
		fail("at must be a non-negative integer timestamp")
	end
	return value
end

local function validate(opts)
	if type(opts) ~= "table" then
		fail("apply requires options")
	end
	for key in pairs(opts) do
		if key ~= "run" and key ~= "store" and key ~= "baseline" and key ~= "override" and key ~= "at" then
			fail("apply contains unsupported field: " .. tostring(key))
		end
	end
	if not core_run.is(opts.run) then
		fail("run must be a persistent Gator run")
	end
	if type(opts.store) ~= "table" or type(opts.store.put) ~= "function" then
		fail("store must persist the run audit event")
	end
	if not overlay.is(opts.baseline) then
		fail("baseline must be a policy overlay")
	end
	if not overlay.is(opts.override) or opts.override.scope ~= "run" then
		fail("override must be a run policy overlay")
	end
	if opts.override.target ~= opts.run.id then
		fail("override target must match run id")
	end
	if opts.override.provenance.source ~= "run-override" or opts.override.provenance.ref ~= opts.run.id then
		fail("override provenance must identify the target run")
	end
	if next(opts.override.rules) == nil then
		fail("override must select a narrower rule or mode")
	end
	return timestamp(opts.at or os.time())
end

local function narrower(baseline, override)
	for key, value in pairs(override.rules) do
		if key:match("_allowed$") then
			if type(value) ~= "boolean" then
				fail("override " .. key .. " must be boolean")
			end
			if value and baseline.rules[key] ~= true then
				fail("override " .. key .. " would broaden the baseline")
			end
		elseif key == "mode" then
			if type(value) ~= "string" or modes[value] == nil then
				fail("override mode must be read_only, plan, or default")
			end
			local baseline_mode = baseline.rules.mode or "default"
			if type(baseline_mode) ~= "string" or modes[baseline_mode] == nil then
				fail("baseline mode must be read_only, plan, or default")
			end
			if modes[value] > modes[baseline_mode] then
				fail("override mode would broaden the baseline")
			end
		elseif not vim.deep_equal(value, baseline.rules[key]) then
			fail("override rule is not an approved narrowing: " .. key)
		end
	end
end

local function effective(baseline, override)
	local rules = vim.deepcopy(baseline.rules)
	for key, value in pairs(override.rules) do
		rules[key] = vim.deepcopy(value)
	end
	return overlay.new({
		scope = "run",
		target = override.target,
		rules = rules,
		provenance = vim.deepcopy(override.provenance),
	})
end

local function event(run, baseline, override, policy, at)
	local payload = {
		baseline = { scope = baseline.scope, provenance = vim.deepcopy(baseline.provenance) },
		override = overlay.to_record(override),
		effective = overlay.to_record(policy),
	}
	local id = "policy-override-" .. vim.fn.sha256(run.id .. "\0" .. vim.json.encode(payload) .. "\0" .. at):sub(1, 24)
	return core_run.event({ id = id, run_id = run.id, type = "policy.run_override", at = at, payload = payload })
end

local function duplicate(run, id)
	for _, value in ipairs(run.events) do
		if value.id == id then
			fail("run override has already been applied")
		end
	end
end

function M.apply(opts)
	local at = validate(opts)
	narrower(opts.baseline, opts.override)
	local policy = effective(opts.baseline, opts.override)
	local audit = event(opts.run, opts.baseline, opts.override, policy, at)
	duplicate(opts.run, audit.id)
	local run = core_run.append_event(opts.run, audit)
	return { policy = policy, event = audit, run = opts.store:put(run) }
end

return M
