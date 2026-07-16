local contracts = require("gator").module("extensions").contracts
local report = contracts.verify()
assert(report.api_version == 1 and vim.deep_equal(report.modules, {
	"adapter_sdk",
	"events",
	"policy_sdk",
	"retrieval_sdk",
	"ui_sdk",
	"workflow_sdk",
}), "contract suite must verify every public extension API")
assert(
	vim.deep_equal(contracts.verify({ only = { "events", "workflow_sdk" } }).modules, { "events", "workflow_sdk" }),
	"contract suite must support deterministic focused verification"
)
assert(not pcall(contracts.verify, { only = { "missing" } }), "unknown public APIs must fail explicitly")
