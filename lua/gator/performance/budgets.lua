local M = {
	schema_version = 1,
	samples = 3,
	values = {
		startup = { duration_ms = 200, memory_kb_delta = 16384, variance_percent = 100, sustained_samples = 3 },
		context = { duration_ms = 200, memory_kb_delta = 16384, variance_percent = 100, sustained_samples = 3 },
		stream = { duration_ms = 800, memory_kb_delta = 32768, variance_percent = 100, sustained_samples = 3 },
		worktree = { duration_ms = 250, memory_kb_delta = 16384, variance_percent = 100, sustained_samples = 3 },
		diff = { duration_ms = 800, memory_kb_delta = 32768, variance_percent = 100, sustained_samples = 3 },
		indexer = { duration_ms = 500, memory_kb_delta = 32768, variance_percent = 100, sustained_samples = 3 },
	},
}

return M
