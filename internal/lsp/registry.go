package lsp

import (
	"errors"
	"sync"
	"time"
)

const defaultRegistryManagers = 8

// Registry retains trusted language-server processes for a single native Gator
// session. Entries are scoped to one exact worktree and configuration hash; it
// is intentionally in-memory only and must be closed with the owning session.
type Registry struct {
	mu      sync.Mutex
	limit   int
	entries map[string]*registryEntry
	closed  bool
	now     func() time.Time
}

type registryEntry struct {
	root    string
	hash    string
	manager *Manager
	active  int
	retired bool
	usedAt  time.Time
}

// NewRegistry constructs the bounded, session-local language-server cache.
func NewRegistry() *Registry {
	return newRegistry(defaultRegistryManagers)
}

func newRegistry(limit int) *Registry {
	if limit < 1 {
		limit = defaultRegistryManagers
	}
	return &Registry{
		limit:   limit,
		entries: make(map[string]*registryEntry),
		now:     time.Now,
	}
}

// Reconcile retires cached managers for a worktree when its manifest is
// removed, changed, or no longer explicitly trusted. In-flight runs retain
// their manager until release; it cannot be acquired by a later run.
func (r *Registry) Reconcile(set Set) {
	if r == nil || set.root.Path() == "" {
		return
	}
	r.mu.Lock()
	closing := r.reconcileLocked(set)
	r.mu.Unlock()
	closeManagers(closing)
}

// Acquire returns the trusted manager for this exact configuration and a
// release function. Call release once the run ends; the manager stays cached
// for a compatible later native TUI or app-server run.
func (r *Registry) Acquire(set Set) (*Manager, func(), error) {
	if r == nil {
		return nil, nil, errors.New("LSP registry is not configured")
	}
	if !set.trusted || set.configuredHash == "" || set.root.Path() == "" {
		return nil, nil, errors.New("LSP registry requires a trusted configured set")
	}

	r.mu.Lock()
	closing := r.reconcileLocked(set)
	if r.closed {
		r.mu.Unlock()
		closeManagers(closing)
		return nil, nil, errors.New("LSP registry is closed")
	}
	key := registryKey(set.root.Path(), set.configuredHash)
	entry := r.entries[key]
	if entry == nil {
		for len(r.entries) >= r.limit {
			evictedKey, evicted := r.oldestIdleLocked()
			if evicted == nil {
				r.mu.Unlock()
				closeManagers(closing)
				return nil, nil, errors.New("LSP registry has no idle manager to evict")
			}
			delete(r.entries, evictedKey)
			closing = append(closing, evicted.manager)
		}
		entry = &registryEntry{
			root:    set.root.Path(),
			hash:    set.configuredHash,
			manager: set.NewManager(),
			usedAt:  r.now(),
		}
		r.entries[key] = entry
	}
	entry.active++
	entry.usedAt = r.now()
	manager := entry.manager
	r.mu.Unlock()
	closeManagers(closing)

	var once sync.Once
	release := func() {
		once.Do(func() {
			var closeManager *Manager
			r.mu.Lock()
			current := r.entries[key]
			if current == entry && current.active > 0 {
				current.active--
				current.usedAt = r.now()
				if current.retired && current.active == 0 {
					delete(r.entries, key)
					closeManager = current.manager
				}
			}
			r.mu.Unlock()
			if closeManager != nil {
				_ = closeManager.Close()
			}
		})
	}
	return manager, release, nil
}

// Close stops every cached manager, including a manager held by an in-flight
// run. The owning TUI or app-server calls this only during session shutdown.
func (r *Registry) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	closing := make([]*Manager, 0, len(r.entries))
	for _, entry := range r.entries {
		closing = append(closing, entry.manager)
	}
	r.entries = make(map[string]*registryEntry)
	r.mu.Unlock()
	closeManagers(closing)
}

func (r *Registry) reconcileLocked(set Set) []*Manager {
	closing := make([]*Manager, 0)
	for key, entry := range r.entries {
		if entry.root != set.root.Path() {
			continue
		}
		if set.trusted && set.configuredHash == entry.hash {
			continue
		}
		entry.retired = true
		if entry.active == 0 {
			delete(r.entries, key)
			closing = append(closing, entry.manager)
		}
	}
	return closing
}

func (r *Registry) oldestIdleLocked() (string, *registryEntry) {
	var (
		oldestKey string
		oldest    *registryEntry
	)
	for key, entry := range r.entries {
		if entry.active != 0 {
			continue
		}
		if oldest == nil || entry.usedAt.Before(oldest.usedAt) {
			oldestKey, oldest = key, entry
		}
	}
	return oldestKey, oldest
}

func registryKey(root, hash string) string {
	return root + "\x00" + hash
}

func closeManagers(managers []*Manager) {
	for _, manager := range managers {
		if manager != nil {
			_ = manager.Close()
		}
	}
}
