package goal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/filelock"
)

// ReviewDigest identifies the saved responsibility, not an authority grant.
// A changed outcome, workspace, linked chat, status or notification policy needs
// a fresh review before a manual status control can apply.
func (r Record) ReviewDigest() (string, error) {
	if err := validate(r); err != nil {
		return "", err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (s Store) SetStatusReviewed(id string, status Status, expected string) (Record, error) {
	if status != Active && status != Paused {
		return Record{}, errors.New("reviewed goal controls may pause or resume only")
	}
	if len(expected) != 64 || strings.ToLower(expected) != expected {
		return Record{}, errors.New("review the current goal before changing its status")
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return Record{}, errors.New("invalid goal review identity")
	}
	return s.changeRecord(id, expected, func(record *Record) error {
		if record.Status == Completed {
			return errors.New("completed goals cannot be resumed from Following")
		}
		record.Status = status
		return nil
	})
}

func (s Store) lockRecord(id string) (func(), error) {
	if !validID(id) {
		return nil, errors.New("invalid goal ID")
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := filelock.OpenRootFile(root, ".record-"+id+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := filelock.Lock(file, filelock.Exclusive); err != nil {
		file.Close()
		return nil, err
	}
	return func() { _ = filelock.Unlock(file); _ = file.Close() }, nil
}

func (s Store) changeRecord(id, expected string, change func(*Record) error) (Record, error) {
	unlock, err := s.lockRecord(id)
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if expected != "" {
		digest, err := record.ReviewDigest()
		if err != nil {
			return Record{}, err
		}
		if digest != expected {
			return Record{}, errors.New("this goal changed; refresh Following and review it again")
		}
	}
	if err := change(&record); err != nil {
		return Record{}, err
	}
	return s.save(record)
}
