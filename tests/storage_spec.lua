local storage = require("gator").module("core").storage

local backend = { api_version = 1 }
for _, name in ipairs(storage.json_contract().methods) do
	backend[name] = function() end
end
assert(storage.validate_json_backend(backend) == backend, "JSON storage contracts must expose every versioned method")
backend.append_run = nil
assert(not pcall(storage.validate_json_backend, backend), "JSON storage contracts must reject incomplete backends")

local filesystem = require("gator").module("core").filesystem
local files = {}
local selected = storage.resolve({
	kind = "json",
	path = "/fixture/state.json",
	filesystem = filesystem.new({
		readable = function(path)
			return files[path] ~= nil
		end,
		read = function(path)
			return files[path]
		end,
		mkdir = function()
			return true
		end,
		write = function(path, value)
			files[path] = value
			return true
		end,
		rename = function(source, target)
			files[target], files[source] = files[source], nil
			return true
		end,
		remove = function(path)
			files[path] = nil
			return true
		end,
	}),
})
assert(
	selected.kind == "json" and selected.backend.api_version == 1,
	"storage resolution must select the injected JSON backend"
)
assert(not pcall(storage.resolve, { kind = "missing" }), "storage resolution must reject unavailable backends")
