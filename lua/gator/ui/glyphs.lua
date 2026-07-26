local M = {}

local styles = {
	unicode = {
		brand = "🐊",
		task = "📋",
		session = "🔗",
		context = "📎",
		review = "🔎",
		provider = "🤖",
		workspace = "📁",
		info = "ℹ",
		success = "✅",
		warning = "⚠",
		error = "✖",
		empty = "∅",
		wait = "⏳",
		keymap = "⌨",
		selected = "🐊",
	},
	nerd_font = {
		brand = "🐊",
		task = "󰄬",
		session = "",
		context = "󰆨",
		review = "󰄬",
		provider = "󰚩",
		workspace = "",
		info = "",
		success = "",
		warning = "",
		error = "",
		empty = "∅",
		wait = "",
		keymap = "",
		selected = "",
	},
	ascii = {
		brand = "[G]",
		task = "[T]",
		session = "[S]",
		context = "[C]",
		review = "[R]",
		provider = "[P]",
		workspace = "[W]",
		info = "[i]",
		success = "[+]",
		warning = "[!]",
		error = "[x]",
		empty = "[-]",
		wait = "[...]",
		keymap = "[keys]",
		selected = "*",
	},
}
local style = "unicode"

local function fail(message)
	error("Gator UI glyphs: " .. message, 3)
end

local function icon(name)
	return styles[style][name] or styles[style].brand
end

local function prefix(value, name)
	return icon(name) .. " " .. value
end

local function status(line)
	local value = line:match("^State:%s*([%a_]+)")
	if value == "ready" or value == "completed" or value == "succeeded" then
		return "success"
	end
	if value == "loading" or value == "recovering" or value == "pending" then
		return "wait"
	end
	if value == "degraded" or value == "unavailable" then
		return "warning"
	end
	if value == "failed" or value == "error" then
		return "error"
	end
	return "info"
end

function M.configure(value)
	if type(value) ~= "string" or not styles[value] then
		fail("style must be unicode, nerd_font, or ascii")
	end
	style = value
	return style
end

function M.style()
	return style
end

function M.get(name)
	if type(name) ~= "string" or name == "" then
		fail("icon name must be non-empty text")
	end
	return icon(name)
end

function M.decorate(lines, filetype)
	if type(lines) ~= "table" or not vim.islist(lines) then
		fail("lines must be an array")
	end
	if type(filetype) ~= "string" or filetype == "" then
		fail("filetype must be non-empty text")
	end
	if filetype == "gator-markdown" then
		return vim.deepcopy(lines)
	end
	local result = {}
	for index, source in ipairs(lines) do
		if type(source) ~= "string" then
			fail("line " .. index .. " must be text")
		end
		local line = source
		if index == 1 then
			line = prefix(line, "brand")
		elseif line:match("^State:") then
			line = prefix(line, status(line))
		elseif line:match("^Tasks:") then
			line = prefix(line, "task")
		elseif line:match("^Sessions:") then
			line = prefix(line, "session")
		elseif line:match("^Context:") then
			line = prefix(line, "context")
		elseif line:match("^Review:") or line:match("^Validation:") then
			line = prefix(line, "review")
		elseif line:match("^Provider:") or line:match("^Providers:") or line:match("^Provider ") then
			line = prefix(line, "provider")
		elseif line:match("^Workspace:") or line:match("^Layout:") then
			line = prefix(line, "workspace")
		elseif line:match("^No ") then
			line = prefix(line, "empty")
		elseif line:match("^Waiting") then
			line = prefix(line, "wait")
		elseif line:match("^Degraded:") then
			line = prefix(line, "warning")
		elseif line:find("navigate", 1, true) and line:find("help", 1, true) then
			line = prefix(line, "keymap")
		end
		result[index] = line
	end
	return result
end

return M
