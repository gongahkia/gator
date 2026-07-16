local sdk = require("gator").module("extensions").retrieval_sdk
local cloud = sdk.define({
	name = "cloud-fixture",
	kind = "cloud",
	retrieve = function()
		return { { ref = "cloud://one", content = "result", score = 1 } }
	end,
})
assert(not pcall(cloud.retrieve, { query = "needle" }), "cloud retrieval must require explicit privacy consent")
local value = cloud.retrieve({ query = "needle", privacy_consent = true })
assert(
	value[1].provider == "cloud-fixture" and value[1].ref == "cloud://one",
	"retrieval SDK must retain provider provenance"
)
local lexical = sdk.define({
	name = "lexical-fixture",
	kind = "lexical",
	retrieve = function()
		return { { ref = "file://one" } }
	end,
})
assert(
	lexical.retrieve({ query = "needle" })[1].ref == "file://one",
	"local and lexical providers must not require cloud consent"
)
assert(not pcall(sdk.define, {
	name = "bad",
	kind = "unknown",
	retrieve = function()
		return {}
	end,
}), "unsupported retrieval providers must fail explicitly")
