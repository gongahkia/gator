local auth = require("gator").module("adapters").auth
local logged_in = auth.discover({
	provider = "codex",
	probe = function()
		return { authenticated = true }
	end,
})
assert(
	logged_in.provider == "codex" and logged_in.authenticated,
	"authentication discovery must report CLI-owned login state"
)
local unavailable = auth.discover({
	provider = "codex",
	probe = function()
		return { authenticated = false, reason = "login required" }
	end,
})
assert(
	not unavailable.authenticated and unavailable.reason == "login required",
	"unauthenticated CLIs must expose their reason"
)
local failed = auth.discover({
	provider = "codex",
	probe = function()
		error("offline")
	end,
})
assert(
	not failed.authenticated and failed.reason == "CLI authentication probe failed",
	"probe failures must not imply authentication"
)
local ok = pcall(auth.discover, {
	provider = "codex",
	probe = function()
		return { authenticated = true }
	end,
	token = "secret",
})
assert(not ok, "authentication discovery must reject credential fields")
