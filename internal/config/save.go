package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func (cfg Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return toml.NewEncoder(file).Encode(cfg)
}
