local storage = require("gator").module("core").storage

local backend = { api_version = 1 }
for _, name in ipairs(storage.json_contract().methods) do
	backend[name] = function() end
end
assert(storage.validate_json_backend(backend) == backend, "JSON storage contracts must expose every versioned method")
backend.append_run = nil
assert(not pcall(storage.validate_json_backend, backend), "JSON storage contracts must reject incomplete backends")
