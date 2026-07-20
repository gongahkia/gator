local config = require("gator.config")
local state = require("gator.state")

local value = state.new(config.resolve(), { supported = true })
local events = {}
local subscription = value:subscribe(function(snapshot, event)
	table.insert(events, { snapshot = snapshot, event = event })
end)

assert(state.is(value), "state must expose a typed reactive store")
assert(value:version() == 0, "new state stores must start at version zero")
assert(
	value:update({ workspace = { status = "loading", detail = "refreshing" } }).workspace.status == "loading",
	"state updates must return validated snapshots"
)
assert(
	value.workspace.status == "loading" and value:version() == 1 and events[1].event.changed[1] == "workspace",
	"state updates must notify subscribers with deterministic change metadata"
)

value:mutate(function(next)
	next.context.selections = { { id = "selection-1" } }
end)
assert(value.context.selections[1].id == "selection-1", "state mutations must commit nested changes atomically")

local snapshot = value:snapshot()
snapshot.workspace.status = "failed"
assert(value.workspace.status == "loading", "state snapshots must not expose mutable store internals")
assert(subscription:cancel(), "state subscriptions must support explicit cancellation")
assert(not subscription:cancel(), "state cancellation must be idempotent")
value.adapters = { codex = { available = false } }
assert(#events == 2, "cancelled subscriptions must not receive later state changes")

assert(not pcall(value.update, value, { missing = true }), "state updates must reject unknown fields")
assert(not pcall(value.subscribe, value, true), "state subscriptions must require callbacks")
