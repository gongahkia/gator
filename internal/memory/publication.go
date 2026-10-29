package memory

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/gongahkia/gator/internal/privatefs"
)

func privateDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("memory path must be an ordinary directory: %s", path)
	}
	return privatefs.Protect(path)
}

// Publish only fully written, synced JSON. Creation uses an exclusive link so
// an ID collision cannot replace an existing record. Interrupted temporary files
// remain hidden from readers. All callers hold the store's mutation lock.
func (s Store) write(record Record, create bool) error {
	if err := validate(record); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	name := ".memory-" + hex.EncodeToString(nonce[:])
	temp, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	if _, err := temp.Write(data); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Sync(); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if create {
		err = root.Link(name, record.ID+".json")
	} else {
		err = root.Rename(name, record.ID+".json")
	}
	if err != nil {
		return err
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return privatefs.SyncDir(directory)
}
