local transfer = require("gator").module("context").transfer
local manual = transfer.summary({
	mode = "manual",
	source_provider = "codex",
	target_provider = "claude",
	content = "summary",
	confirmed = true,
})
assert(manual.available and manual.editable, "manual summaries must remain editable after confirmation")
local explicit = transfer.summary({
	mode = "explicit",
	source_provider = "codex",
	target_provider = "gemini",
	content = "summary",
	confirmed = true,
})
assert(explicit.available and explicit.editable, "explicit summaries must remain editable")
local blocked =
	transfer.summary({ mode = "automatic", source_provider = "codex", target_provider = "pi", content = "summary" })
assert(not blocked.available, "automatic summaries must require opt-in")
local automatic = transfer.summary({
	mode = "automatic",
	source_provider = "codex",
	target_provider = "pi",
	content = "summary",
	opt_in = true,
})
assert(automatic.available and not automatic.editable, "opted-in automatic summaries must remain explicit records")
