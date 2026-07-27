local project_policy = require("gator.policy.project")

local M = {}

local function fail(message)
	error("Gator trust: " .. tostring(message), 3)
end

local function field(value, name)
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail(name .. " must be an object")
	end
	return value
end

local function setting(value)
	if value ~= "workspace_write" and value ~= "read_only" then
		fail("permissions.codex.sandbox must be workspace_write or read_only")
	end
	return value
end

local function state(value, detail, mode)
	local result = { state = value, detail = detail }
	if mode then
		result.mode = mode
	end
	return result
end

local function terminal(provider, project)
	local detail = "provider-owned terminal; Gator does not inspect or enforce this"
	return {
		surface = "terminal",
		security_owner = "provider",
		policy = state(project and "not_enforced" or "not_configured", project and "project policy cannot govern a terminal provider" or "no Gator-enforced terminal policy"),
		write = state("provider_owned", detail),
		network = state("provider_owned", detail),
		mcp = state("provider_owned", detail),
		approval = state("provider_owned", detail),
		provider = provider,
	}
end

local function managed(provider, project)
	return {
		surface = "structured",
		security_owner = "provider",
		policy = state(project and "not_enforced" or "not_configured", project and "project policy is not enforceable for this provider" or "no Gator-enforced policy for this provider"),
		write = state("unknown", "provider controls filesystem access"),
		network = state("unknown", "provider controls network access"),
		mcp = state("unknown", "provider controls MCP access"),
		approval = state("when_emitted", "Gator renders a provider approval request only when the provider emits one"),
		provider = provider,
	}
end

local function pi(project)
	return {
		surface = "structured",
		security_owner = "provider",
		policy = state(project and "not_enforced" or "not_configured", project and "project policy is not enforceable for Pi RPC" or "no Gator-enforced Pi policy"),
		write = state("unknown", "Pi controls filesystem access"),
		network = state("unknown", "Pi controls network access"),
		mcp = state("unknown", "Pi controls MCP access"),
		approval = state("unavailable", "Pi RPC does not expose a Gator approval bridge"),
		provider = "pi",
	}
end

local function codex(mode, source)
	local app_server_mode = mode == "read_only" and "readOnly" or "workspaceWrite"
	return {
		surface = "structured",
		security_owner = "provider",
		policy = state("applied", "Codex App Server launch policy", source),
		write = state("codex_enforced", "Codex App Server sandbox", mode),
		network = state("unknown", "not controlled by Gator"),
		mcp = state("unknown", "not controlled by Gator"),
		approval = state("on_request", "Gator renders Codex approval requests"),
		provider = "codex",
		codex = { sandbox = app_server_mode, approval_policy = "on-request" },
	}
end

local function rules(value)
	if not value then
		return nil
	end
	field(value, "project policy rules")
	for key in pairs(value) do
		if key ~= "write_allowed" then
			fail("project policy rule cannot be enforced safely: " .. tostring(key))
		end
	end
	if type(value.write_allowed) ~= "boolean" then
		fail("project policy requires explicit boolean write_allowed")
	end
	return value
end

function M.resolve(opts)
	if type(opts) ~= "table" then
		fail("resolve requires options")
	end
	for key in pairs(opts) do
		if key ~= "provider" and key ~= "transport" and key ~= "root" and key ~= "config" and key ~= "project_policy" then
			fail("resolve contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.provider) ~= "string" or opts.provider == "" then
		fail("provider must be non-empty text")
	end
	if opts.transport ~= "terminal" and opts.transport ~= "chat" then
		fail("transport must be terminal or chat")
	end
	field(opts.config, "config")
	field(opts.config.permissions, "config.permissions")
	field(opts.config.permissions.codex, "config.permissions.codex")
	local policy = opts.project_policy
	if policy == nil and opts.root then
		policy = project_policy.load({ cwd = opts.root })
	end
	if policy ~= nil and type(policy) ~= "table" then
		fail("project_policy must be an object")
	end
	local project_rules = policy and policy.available and rules(policy.policy and policy.policy.rules) or nil
	if opts.transport == "terminal" then
		if project_rules and not project_rules.write_allowed then
			fail("project policy requires read-only; terminal providers are provider-owned and cannot enforce it")
		end
		return terminal(opts.provider, project_rules ~= nil)
	end
	if opts.provider == "codex" then
		local mode = setting(opts.config.permissions.codex.sandbox)
		local source = "gator.config"
		if project_rules then
			mode = project_rules.write_allowed and "workspace_write" or "read_only"
			source = "project-policy"
		end
		return codex(mode, source)
	end
	if project_rules and not project_rules.write_allowed then
		fail("project policy requires read-only; " .. opts.provider .. " exposes no Gator-enforced read-only structured control")
	end
	if opts.provider == "pi" then
		return pi(project_rules ~= nil)
	end
	return managed(opts.provider, project_rules ~= nil)
end

local function detail(value, name)
	field(value, name)
	if type(value.state) ~= "string" or value.state == "" or type(value.detail) ~= "string" or value.detail == "" then
		fail(name .. " must declare state and detail")
	end
	if value.mode ~= nil and (type(value.mode) ~= "string" or value.mode == "") then
		fail(name .. ".mode must be non-empty text")
	end
	return value
end

function M.normalize(value, legacy)
	if value == nil and legacy then
		return {
			surface = legacy.transport == "terminal" and "terminal" or "structured",
			security_owner = "provider",
			policy = state("unknown", "legacy run; launch trust was not recorded"),
			write = state("unknown", "legacy run; launch trust was not recorded"),
			network = state("unknown", "legacy run; launch trust was not recorded"),
			mcp = state("unknown", "legacy run; launch trust was not recorded"),
			approval = state("unknown", "legacy run; launch trust was not recorded"),
			provider = legacy.provider,
		}
	end
	field(value, "run.trust")
	for key in pairs(value) do
		if key ~= "surface" and key ~= "security_owner" and key ~= "policy" and key ~= "write" and key ~= "network" and key ~= "mcp" and key ~= "approval" and key ~= "provider" then
			fail("run.trust contains unsupported field: " .. tostring(key))
		end
	end
	if (value.surface ~= "terminal" and value.surface ~= "structured") or value.security_owner ~= "provider" then
		fail("run.trust surface or security_owner is unavailable")
	end
	if type(value.provider) ~= "string" or value.provider == "" then
		fail("run.trust.provider must be non-empty text")
	end
	for _, name in ipairs({ "policy", "write", "network", "mcp", "approval" }) do
		detail(value[name], "run.trust." .. name)
	end
	return vim.deepcopy(value)
end

function M.summary(value)
	value = M.normalize(value)
	return string.format(
		"%s · write %s%s · network %s · MCP %s · approval %s",
		value.surface == "terminal" and "terminal" or "structured chat",
		value.write.state,
		value.write.mode and (" (" .. value.write.mode .. ")") or "",
		value.network.state,
		value.mcp.state,
		value.approval.state
	)
end

return M
