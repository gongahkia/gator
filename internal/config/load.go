package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

func Defaults() Config {
	return Config{
		Brain: EndpointConfig{
			Transport: "ollama",
			BaseURL:   "http://localhost:11434",
			Model:     "gpt-oss:20b",
		},
		Drone: EndpointConfig{
			Transport: "ollama",
			BaseURL:   "http://localhost:11434",
			Model:     "qwen3:8b",
		},
		MaxTurns:       40,
		MaxBrainTokens: 200000,
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	explicit := path != ""
	if path == "" {
		path = DefaultPath()
	}
	if err := loadFile(path, explicit, &cfg); err != nil {
		return Config{}, err
	}
	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "paw", "config.toml")
}

func loadFile(path string, explicit bool, cfg *Config) error {
	if path == "" {
		return nil
	}
	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return nil
		}
		return err
	}
	_, err = toml.DecodeFile(path, cfg)
	return err
}

func applyEnv(cfg *Config) error {
	setString("PAW_BRAIN_TRANSPORT", &cfg.Brain.Transport)
	setString("PAW_BRAIN_BASE_URL", &cfg.Brain.BaseURL)
	setString("PAW_BRAIN_API_KEY", &cfg.Brain.APIKey)
	setString("PAW_BRAIN_MODEL", &cfg.Brain.Model)
	setString("PAW_DRONE_TRANSPORT", &cfg.Drone.Transport)
	setString("PAW_DRONE_BASE_URL", &cfg.Drone.BaseURL)
	setString("PAW_DRONE_API_KEY", &cfg.Drone.APIKey)
	setString("PAW_DRONE_MODEL", &cfg.Drone.Model)
	if err := setInt("PAW_MAX_TURNS", &cfg.MaxTurns); err != nil {
		return err
	}
	if err := setInt("PAW_MAX_BRAIN_TOKENS", &cfg.MaxBrainTokens); err != nil {
		return err
	}
	if err := setDuration("PAW_CALL_TIMEOUT", &cfg.CallTimeout); err != nil {
		return err
	}
	if err := setInt("PAW_GATHER_MAX_DEPTH", &cfg.Gather.MaxDepth); err != nil {
		return err
	}
	return setInt("PAW_GATHER_MAX_FILE_BYTES", &cfg.Gather.MaxFileBytes)
}

func setString(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func setInt(key string, dst *int) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	*dst = n
	return nil
}

func setDuration(key string, dst *time.Duration) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	*dst = d
	return nil
}
