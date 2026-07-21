local errors = require("gator").module("core").error_event

local value = errors.new({
	id = "event-error",
	run_id = "run-error",
	provider = { name = "codex", session_id = "native-error" },
	sequence = 0,
	at = 1,
	kind = "transport",
	message = "token: private-value",
	retryable = true,
})
assert(
	value.type == "run.error"
		and value.payload.message:find("private%-value") == nil
		and value.provider.session_id == "native-error",
	"provider errors must preserve session identity while redacting diagnostics"
)
assert(not pcall(errors.new, {
	id = "event-error-invalid",
	run_id = "run-error",
	provider = { name = "codex" },
	sequence = 1,
	at = 2,
	kind = "missing",
	message = "error",
	retryable = false,
}), "provider errors must reject unavailable classifications")
