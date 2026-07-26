local actions = require("gator.ui.actions")
local invoked = {}

assert(actions.register({
	name = "open-dashboard",
	label = "Open dashboard",
	execute = function()
		table.insert(invoked, "action")
	end,
}) == "open-dashboard", "workspace action registration must expose action names")
assert(actions.list()[1].label == "Open dashboard", "workspace actions must retain their display label")
actions.list()[1].execute()
assert(invoked[1] == "action", "workspace actions must retain executable callbacks")
assert(actions.unregister("open-dashboard"), "workspace actions must be removable")
assert(not pcall(actions.register, { name = "bad", execute = true }), "workspace actions require executable callbacks")
