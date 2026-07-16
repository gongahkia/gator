local M = {}

local function fail(message)
  error("Gator adapter fixture: " .. message, 3)
end

local function read(path)
  local file, err = io.open(path, "rb")
  if not file then fail(err) end
  local content = file:read("*a")
  file:close()
  return content
end

local function decode(path)
  local ok, value = pcall(vim.json.decode, read(path))
  if not ok then fail("invalid JSON in " .. path .. ": " .. value) end
  if type(value) ~= "table" then fail("JSON fixture must contain an object or array: " .. path) end
  return value
end

local function require_callback(callback, name)
  if type(callback) ~= "function" then fail(name .. " callback must be a function") end
end

local function require_list(value, name)
  if type(value) ~= "table" or not vim.islist(value) then fail(name .. " must be an array") end
  return value
end

local function validate_jsonrpc(message, index)
  if message.jsonrpc ~= "2.0" then fail("JSON-RPC record " .. index .. " must declare jsonrpc 2.0") end
  local request = message.method ~= nil
  local has_result = message.result ~= nil
  local has_error = message.error ~= nil

  if request then
    if type(message.method) ~= "string" then fail("JSON-RPC method at record " .. index .. " must be a string") end
    if has_result or has_error then fail("JSON-RPC request at record " .. index .. " cannot include a response") end
    return
  end

  if message.id == nil then fail("JSON-RPC response at record " .. index .. " must include an id") end
  if has_result == has_error then fail("JSON-RPC response at record " .. index .. " must include exactly one result or error") end
  if has_error then
    if type(message.error) ~= "table" then fail("JSON-RPC error at record " .. index .. " must be an object") end
    if type(message.error.code) ~= "number" or message.error.code % 1 ~= 0 then
      fail("JSON-RPC error code at record " .. index .. " must be an integer")
    end
    if type(message.error.message) ~= "string" then fail("JSON-RPC error message at record " .. index .. " must be a string") end
  end
end

function M.replay_jsonl(path, callback)
  require_callback(callback, "JSONL")
  local content = read(path)
  if content == "" then return 0 end
  local lines = vim.split(content, "\n", { plain = true, trimempty = false })

  if lines[#lines] == "" and content:sub(-1) == "\n" then table.remove(lines) end
  for index, line in ipairs(lines) do
    line = line:gsub("\r$", "")
    if line == "" then fail("blank JSONL record at line " .. index .. " in " .. path) end
    local ok, value = pcall(vim.json.decode, line)
    if not ok then fail("invalid JSONL record at line " .. index .. " in " .. path .. ": " .. value) end
    if type(value) ~= "table" or vim.islist(value) then fail("JSONL record at line " .. index .. " must be an object") end
    callback(value, index)
  end
  return #lines
end

function M.replay_jsonrpc(path, callback)
  require_callback(callback, "JSON-RPC")
  return M.replay_jsonl(path, function(message, index)
    validate_jsonrpc(message, index)
    callback(message, index)
  end)
end

function M.replay_terminal(path, callback)
  require_callback(callback, "terminal")
  local chunks = require_list(decode(path).chunks, "terminal fixture chunks")
  for index, chunk in ipairs(chunks) do
    if type(chunk) ~= "string" then fail("terminal chunk " .. index .. " must be a string") end
    callback(chunk, index)
  end
  return #chunks
end

function M.replay_process(path, callbacks)
  if type(callbacks) ~= "table" then fail("process callbacks must be a table") end
  if callbacks.stdout ~= nil then require_callback(callbacks.stdout, "process stdout") end
  if callbacks.stderr ~= nil then require_callback(callbacks.stderr, "process stderr") end
  if callbacks.exit ~= nil then require_callback(callbacks.exit, "process exit") end
  local fixture = decode(path)
  local stdout = require_list(fixture.stdout, "process fixture stdout")
  local stderr = require_list(fixture.stderr, "process fixture stderr")
  if type(fixture.code) ~= "number" or fixture.code % 1 ~= 0 or fixture.code < 0 then
    fail("process fixture code must be a non-negative integer")
  end

  for index, chunk in ipairs(stdout) do
    if type(chunk) ~= "string" then fail("process stdout chunk " .. index .. " must be a string") end
    if callbacks.stdout then callbacks.stdout(chunk, index) end
  end
  for index, chunk in ipairs(stderr) do
    if type(chunk) ~= "string" then fail("process stderr chunk " .. index .. " must be a string") end
    if callbacks.stderr then callbacks.stderr(chunk, index) end
  end

  local status = { code = fixture.code }
  if callbacks.exit then
    callbacks.exit(status)
  end
  return status
end

return M
