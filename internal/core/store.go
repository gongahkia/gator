package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	root string
	dir  string
	mu   sync.Mutex
}

func NewStore(root string) *Store {
	return &Store{root: root, dir: filepath.Join(root, StandaloneStatePath)}
}

func (s *Store) Root() string { return s.root }
func (s *Store) Dir() string  { return s.dir }

func (s *Store) Ensure() error {
	for _, directory := range []string{
		s.dir,
		filepath.Join(s.dir, "tasks"),
		filepath.Join(s.dir, "runs"),
		filepath.Join(s.dir, "events"),
		filepath.Join(s.dir, "evidence"),
		filepath.Join(s.dir, "episodes"),
		filepath.Join(s.dir, "experiments"),
	} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("create Gator local state: %w", err)
		}
	}
	return s.ensureGitExclude()
}

func (s *Store) ensureGitExclude() error {
	gitDir, err := runGit(contextBackground(), s.root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	path := strings.TrimSpace(gitDir)
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.root, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !containsLine(string(data), ".gator/") {
		return os.WriteFile(path, append(data, []byte("\n.gator/\n")...), 0o600)
	}
	return nil
}

func containsLine(text, target string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
}

func contextBackground() context.Context { return context.Background() }

func atomicWrite(path string, data []byte, permission os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".gator-*-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(permission); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (s *Store) PutRun(run Run) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return atomicWrite(filepath.Join(s.dir, "runs", run.ID+".json"), data, 0o600)
}

func (s *Store) GetRun(id string) (Run, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "runs", id+".json"))
	if err != nil {
		return Run{}, fmt.Errorf("read run %s: %w", id, err)
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return Run{}, fmt.Errorf("decode run %s: %w", id, err)
	}
	if run.SchemaVersion != SchemaVersion || run.ID != id {
		return Run{}, fmt.Errorf("run %s has an unsupported schema", id)
	}
	return run, nil
}

func (s *Store) ListRuns() ([]Run, error) {
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(s.dir, "runs", "*.json"))
	if err != nil {
		return nil, err
	}
	runs := make([]Run, 0, len(paths))
	for _, path := range paths {
		id := strings.TrimSuffix(filepath.Base(path), ".json")
		run, err := s.GetRun(id)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	return runs, nil
}

func (s *Store) AppendEvent(event Event) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.OpenFile(filepath.Join(s.dir, "events", event.RunID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (s *Store) Events(runID string) ([]Event, error) {
	file, err := os.Open(filepath.Join(s.dir, "events", runID+".jsonl"))
	if os.IsNotExist(err) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	var events []Event
	for {
		line, err := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var event Event
			if decodeErr := json.Unmarshal(line, &event); decodeErr != nil {
				return nil, fmt.Errorf("decode event journal %s: %w", runID, decodeErr)
			}
			events = append(events, event)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return events, nil
}

func (s *Store) PutEvidence(value Evidence) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.dir, "evidence", value.RunID, value.ID+".json"), data, 0o600)
}

func (s *Store) PutEpisode(value Episode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.dir, "episodes", value.ID+".json"), data, 0o600)
}

func (s *Store) PutExperiment(value ExperimentResult) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.dir, "experiments", value.ID+".json"), data, 0o600)
}
