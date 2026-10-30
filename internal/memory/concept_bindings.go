package memory

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const maxConceptBindings = 4096

// Bindings retain only digests of scope/label pairs. They survive forgetting so
// a previously reconciled alias cannot revive a removed concept. No fact text,
// model-generated paraphrase or original subject label is retained here.
type conceptBindings map[string]string

func conceptKey(scope, subject string) string { return historyHash(scope, CanonicalSubject(subject)) }

func (s Store) bindings() (conceptBindings, error) {
	path := filepath.Join(s.root, ".concepts")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return conceptBindings{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("concept bindings must be a bounded ordinary file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bindings conceptBindings
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, err
	}
	if bindings == nil || len(bindings) > maxConceptBindings {
		return nil, errors.New("invalid concept bindings")
	}
	for alias, root := range bindings {
		if !validConceptKey(alias) || !validConceptKey(root) || bindings[root] != root {
			return nil, errors.New("invalid concept binding root")
		}
	}
	return bindings, nil
}

func validConceptKey(key string) bool {
	if len(key) != 64 {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

func (bindings conceptBindings) resolve(scope, subject string) string {
	key := conceptKey(scope, subject)
	if root, ok := bindings[key]; ok {
		return root
	}
	return key
}

func (s Store) blockedConcept(bindings conceptBindings, scope, subject string) (bool, error) {
	root := bindings.resolve(scope, subject)
	keys := map[string]bool{root: true, conceptKey(scope, subject): true}
	// Honor literal markers written before built-in canonicalization existed.
	for alias, target := range conceptAliases {
		if bindings.resolve(scope, target) == root {
			keys[historyHash(scope, alias)] = true
		}
	}
	for alias, target := range bindings {
		if target == root {
			keys[alias] = true
		}
	}
	for key := range keys {
		if _, err := os.Lstat(filepath.Join(s.root, ".forgotten", key)); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}
