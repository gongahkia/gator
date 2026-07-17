local M = {}

function M.create()
	return { available = false, reason = "Aider does not expose provider-owned session creation" }
end

function M.list()
	return { available = false, reason = "Aider chat history files are not a provider session-list API" }
end

function M.resume()
	return { available = false, reason = "Aider restores local chat history files rather than provider session ids" }
end

function M.close()
	return { available = false, reason = "Aider does not expose provider-owned session deletion or close" }
end

return M
