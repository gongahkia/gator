local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local lease = require("gator").module("workspace").lease

local path = helpers.tempdir("workspace-lease") .. "/leases.json"
local store = lease.open(path)
local first = store:acquire({
	worktree = { id = "worktree-one", root = "/tmp/gator-worktree-one" },
	run_id = "run-one",
	at = 100,
	ttl = 30,
})
assert(
	first.state == "completed" and first.lease.run_id == "run-one" and first.writer_lock.worktree_id == "worktree-one",
	"writer lease acquisition must persist a matching writer lock"
)
local restarted = lease.open(path)
local inspected = restarted:inspect({ worktree_id = "worktree-one", at = 101 })
assert(
	inspected.state == "completed" and #inspected.leases == 1 and inspected.writer_lock.run_id == "run-one",
	"leases and writer locks must survive reopening the store"
)
local held = restarted:acquire({
	worktree = { id = "worktree-one", root = "/tmp/gator-worktree-one" },
	run_id = "run-two",
	at = 101,
})
assert(held.state == "unavailable", "an active writer lock must reject another writer")
assert(restarted:acquire({
	worktree = { id = "worktree-one", root = "/tmp/gator-worktree-one" },
	run_id = "run-one",
	at = 110,
	ttl = 40,
}).lease.expires_at == 150, "the lock holder must renew its own lease")
assert(restarted:release({ run_id = "run-one" }).state == "completed", "lease release must remove its writer lock")
assert(
	restarted:inspect({ worktree_id = "worktree-one", at = 111 }).writer_lock == nil,
	"released worktrees must have no active writer lock"
)
assert(restarted:acquire({
	worktree = { id = "worktree-one", root = "/tmp/gator-worktree-one" },
	run_id = "run-one",
	at = 200,
	ttl = 1,
}).state == "completed", "expired-lease fixture must acquire")
assert(restarted:acquire({
	worktree = { id = "worktree-one", root = "/tmp/gator-worktree-one" },
	run_id = "run-two",
	at = 202,
}).state == "completed", "expired writer leases must not block a new writer")
local cancelled = restarted:acquire({
	worktree = { id = "worktree-cancelled", root = "/tmp/gator-worktree-cancelled" },
	run_id = "run-cancelled",
	cancelled = function()
		return true
	end,
})
assert(cancelled.state == "cancelled", "cancelled lease acquisition must not write a lease")
local lock_path = path .. ".lock"
assert(vim.fn.writefile({}, lock_path) == 0, "busy-store fixture must create a lock file")
local busy = restarted:acquire({
	worktree = { id = "worktree-busy", root = "/tmp/gator-worktree-busy" },
	run_id = "run-busy",
})
assert(vim.fn.delete(lock_path) == 0, "busy-store fixture must remove the lock file")
assert(busy.state == "unavailable", "active storage locks must remain explicit")
assert(
	not helpers.read(path):find("provider", 1, true) and not helpers.read(path):find("session", 1, true),
	"lease storage must exclude provider sessions and credentials"
)
