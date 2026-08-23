package terminal

import (
	"errors"
	"sort"
	"sync"
)

// Registry owns the small set of terminal managers that belong to one
// interactive Gator process. It is deliberately in-memory: preserving a PTY
// across application restart requires a separately supervised daemon, not an
// orphaned child process that this package would be unable to recover safely.
type Registry struct {
	mu       sync.Mutex
	managers map[*Manager]struct{}
}

// NewRegistry creates a process-local background-terminal registry.
func NewRegistry() *Registry {
	return &Registry{managers: make(map[*Manager]struct{})}
}

func (r *Registry) register(manager *Manager) {
	if r == nil || manager == nil {
		return
	}
	r.mu.Lock()
	r.managers[manager] = struct{}{}
	r.mu.Unlock()
}

func (r *Registry) unregister(manager *Manager) {
	if r == nil || manager == nil {
		return
	}
	r.mu.Lock()
	delete(r.managers, manager)
	r.mu.Unlock()
}

// Attachment aggregates the restricted developer view from every registered
// manager. It cannot start or detach a process, alter sandbox policy, or
// access terminal input history.
func (r *Registry) Attachment() Attachment {
	return registryAttachment{registry: r}
}

// Close stops all registered tasks. Call it when the interactive Gator
// process exits; it is safe to call multiple times.
func (r *Registry) Close() {
	for _, manager := range r.snapshot() {
		manager.ForceClose()
	}
}

func (r *Registry) snapshot() []*Manager {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	managers := make([]*Manager, 0, len(r.managers))
	for manager := range r.managers {
		managers = append(managers, manager)
	}
	r.mu.Unlock()
	return managers
}

type registryAttachment struct{ registry *Registry }

func (a registryAttachment) List() []Task {
	var tasks []Task
	for _, manager := range a.registry.snapshot() {
		tasks = append(tasks, manager.List()...)
	}
	sort.Slice(tasks, func(first, second int) bool {
		return tasks[first].StartedAt.Before(tasks[second].StartedAt)
	})
	return tasks
}

func (a registryAttachment) Read(id string, cursor int64) (ReadResult, error) {
	manager, err := a.manager(id)
	if err != nil {
		return ReadResult{}, err
	}
	return manager.Read(id, cursor)
}

func (a registryAttachment) Resize(id string, rows, columns int) (Task, error) {
	manager, err := a.manager(id)
	if err != nil {
		return Task{}, err
	}
	return manager.Resize(id, rows, columns)
}

func (a registryAttachment) WriteDeveloper(id string, input []byte) (Task, error) {
	manager, err := a.manager(id)
	if err != nil {
		return Task{}, err
	}
	return manager.WriteDeveloper(id, input)
}

func (a registryAttachment) WriteDeveloperRaw(id string, input []byte) (Task, error) {
	manager, err := a.manager(id)
	if err != nil {
		return Task{}, err
	}
	return manager.WriteDeveloperRaw(id, input)
}

func (a registryAttachment) FlushDeveloperInput(id string) (Task, error) {
	manager, err := a.manager(id)
	if err != nil {
		return Task{}, err
	}
	return manager.FlushDeveloperInput(id)
}

func (a registryAttachment) Stop(id string) (Task, error) {
	manager, err := a.manager(id)
	if err != nil {
		return Task{}, err
	}
	return manager.Stop(id)
}

func (a registryAttachment) manager(id string) (*Manager, error) {
	for _, manager := range a.registry.snapshot() {
		if _, err := manager.lookup(id); err == nil {
			return manager, nil
		}
	}
	return nil, errors.New("terminal task is not available in this Gator session")
}
