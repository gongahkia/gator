package watch

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/fsnotify/fsnotify"
)

const DefaultDebounce = 200 * time.Millisecond

type Runner func(context.Context, Marker) error

type Status struct {
	State  string
	Marker Marker
	Err    error
}

type Watcher struct {
	Root         string
	Markers      []string
	Debounce     time.Duration
	ContextLines int
	Runner       Runner
	Status       func(Status)
	Ready        chan struct{}
}

func (w *Watcher) Run(ctx context.Context) error {
	if w.Runner == nil {
		return fmt.Errorf("watch runner is required")
	}
	root := w.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	matcher, err := LoadIgnore(root)
	if err != nil {
		return err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = watcher.Close() }()
	if err := addTree(watcher, root, matcher); err != nil {
		return err
	}
	if w.Ready != nil {
		close(w.Ready)
		w.Ready = nil
	}
	debounce := w.Debounce
	if debounce <= 0 {
		debounce = DefaultDebounce
	}
	pending := map[string]struct{}{}
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	var timerC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			if err != nil {
				return err
			}
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			changed, err := handleEvent(watcher, matcher, event)
			if err != nil {
				return err
			}
			if changed == "" {
				continue
			}
			pending[changed] = struct{}{}
			if !timer.Stop() && timerC != nil {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounce)
			timerC = timer.C
		case <-timerC:
			timerC = nil
			if err := w.processPending(ctx, root, pending); err != nil {
				return err
			}
			pending = map[string]struct{}{}
		}
	}
}

func addTree(watcher *fsnotify.Watcher, root string, matcher *IgnoreMatcher) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && matcher.Ignored(path, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
}

func handleEvent(watcher *fsnotify.Watcher, matcher *IgnoreMatcher, event fsnotify.Event) (string, error) {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
		return "", nil
	}
	path := filepath.Clean(event.Name)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if matcher.Ignored(path, info.IsDir()) {
		return "", nil
	}
	if info.IsDir() {
		if event.Op&fsnotify.Create != 0 {
			return "", addTree(watcher, path, matcher)
		}
		return "", nil
	}
	if !info.Mode().IsRegular() {
		return "", nil
	}
	return path, nil
}

func (w *Watcher) processPending(ctx context.Context, root string, pending map[string]struct{}) error {
	paths := make([]string, 0, len(pending))
	for path := range pending {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := w.processFile(ctx, root, path); err != nil {
			return err
		}
	}
	return nil
}

func (w *Watcher) processFile(ctx context.Context, root, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	displayPath := path
	if rel, err := filepath.Rel(root, path); err == nil {
		displayPath = filepath.ToSlash(rel)
	}
	markers, err := FindMarkers(path, data, MarkerOptions{
		Markers:      w.Markers,
		ContextLines: w.ContextLines,
		DisplayPath:  displayPath,
	})
	if err != nil {
		return err
	}
	var remove []Marker
	for _, marker := range markers {
		w.emit(Status{State: "found", Marker: marker})
		w.emit(Status{State: "running", Marker: marker})
		if err := w.Runner(ctx, marker); err != nil {
			w.emit(Status{State: "failed", Marker: marker, Err: err})
			continue
		}
		w.emit(Status{State: "passed", Marker: marker})
		remove = append(remove, marker)
	}
	if len(remove) == 0 {
		return nil
	}
	if err := RemoveMarkers(path, remove); err != nil {
		return err
	}
	for _, marker := range remove {
		w.emit(Status{State: "removed", Marker: marker})
	}
	return nil
}

func (w *Watcher) emit(status Status) {
	if w.Status != nil {
		w.Status(status)
	}
}
