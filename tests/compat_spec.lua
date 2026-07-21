local compat = require("gator.compat")

local current = compat.inspect({
	version = { major = 0, minor = 11, patch = 0 },
	capabilities = { health = true, process = true },
})
assert(current.supported, "the minimum supported Neovim version must be accepted")
assert(#current.degraded == 0, "available optional capabilities must not be degraded")
assert(
	vim.deep_equal(compat.summary(current), {
		"Neovim: 0.11.0",
		"Compatibility: supported",
		"Optional capabilities: complete",
	}),
	"compatibility summaries must be stable"
)

local degraded = compat.inspect({
	version = { major = 0, minor = 12, patch = 0 },
	capabilities = { health = false, process = true },
})
assert(vim.deep_equal(degraded.degraded, { "health" }), "missing optional capabilities must be explicit")
assert(compat.summary(degraded)[3] == "Optional capabilities degraded: health", "degradation must be visible")

local manifest = compat.manifest({
	version = { major = 0, minor = 11, patch = 1 },
	capabilities = { health = false, process = true },
})
assert(
	manifest.schema_version == 1
		and manifest.product == "gator"
		and manifest.neovim.minimum.minor == 11
		and manifest.neovim.supported
		and vim.deep_equal(manifest.degraded, { "health" }),
	"compatibility manifests must expose a versioned machine-readable contract"
)
assert(
	vim.deep_equal(
		vim.json.decode(compat.manifest_json({
			version = { major = 0, minor = 11, patch = 1 },
			capabilities = { health = false, process = true },
		})),
		manifest
	),
	"compatibility manifests must publish JSON without changing their contract"
)
local gator = require("gator")
assert(gator.compatibility_manifest({
	version = { major = 0, minor = 11, patch = 1 },
	capabilities = { health = true, process = true },
}).product == "gator", "Gator must publish compatibility manifests through its public API")
assert(not pcall(compat.manifest, "invalid"), "compatibility manifests must reject invalid input")

local unsupported = compat.inspect({
	version = { major = 0, minor = 10, patch = 4 },
	capabilities = { health = true, process = true },
})
local ok, err = pcall(compat.require_supported, unsupported)
assert(not ok, "unsupported Neovim versions must fail before setup")
assert(err:find("Upgrade Neovim to 0.11.0 or newer.", 1, true), "unsupported versions must provide recovery guidance")
