local config = require("gator.config")
local errors = require("gator.error")
local redact = require("gator").module("policy").redact

local redactor = redact.new({ patterns = { "session=[^%s]+" } })
local text = redactor:text(
	"Authorization: Bearer bearer-value api_key=api-value ghp_github-value sk-openai-value session=local-value"
)
assert(
	not text:find("bearer%-value")
		and not text:find("api%-value")
		and not text:find("github%-value")
		and not text:find("openai%-value")
		and not text:find("local%-value"),
	"known and configured credential patterns must redact prompt and log text"
)
local original =
	{ token = "token-value", nested = { authorization = "Bearer nested-value", prompt = "sk-prompt-value" } }
local value = redactor:value(original)
assert(
	value.token == "[REDACTED]"
		and value.nested.authorization == "[REDACTED]"
		and not value.nested.prompt:find("prompt%-value")
		and original.token == "token-value",
	"structured diagnostics and telemetry fields must redact sensitive keys without mutating inputs"
)
local written
local record = redact.log({
	message = "password=log-value",
	fields = { access_key = "access-value", detail = "github_pat_log-value" },
	write = function(value)
		written = value
	end,
})
assert(
	record.message:find("log%-value") == nil
		and record.fields.access_key == "[REDACTED]"
		and written.fields.detail:find("log%-value") == nil,
	"safe log records must only pass redacted values to writers"
)
assert(
	config.resolve({ telemetry = { redaction_patterns = { "private%-[%w]+" } } }).telemetry.redaction_patterns[1]
		== "private%-[%w]+",
	"global settings must retain validated user redaction patterns"
)
assert(
	not pcall(config.resolve, { telemetry = { redaction_patterns = { "a*" } } }),
	"redaction patterns that match empty strings must fail explicitly"
)
redact.configure({ patterns = { "custom%-[%w]+" } })
local formatted = errors.format(errors.new("policy.redaction", "redaction failed", {
	detail = "custom-secret",
	remedy = "remove token=error-value",
}))
assert(
	formatted:find("custom%-secret") == nil and formatted:find("error%-value") == nil,
	"error diagnostics must apply configured redaction before notification"
)
redact.configure({ patterns = {} })
