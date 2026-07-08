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
		Verify: VerifyConfig{
			Timeout: 2 * time.Minute,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	if err := loadFile(DefaultPath(), false, &cfg); err != nil {
		return Config{}, err
	}
	repoPath, err := discoverRepoConfig("")
	if err != nil {
		return Config{}, err
	}
	if repoPath != "" {
		if err := loadFile(repoPath, true, &cfg); err != nil {
			return Config{}, err
		}
	}
	if err := loadFile(path, path != "", &cfg); err != nil {
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

func discoverRepoConfig(cwd string) (string, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	dir, err := canonicalPath(cwd)
	if err != nil {
		return "", err
	}
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home != "" {
		if absHome, err := canonicalPath(home); err == nil {
			home = absHome
		}
	}
	for {
		candidate := filepath.Join(dir, ".paw", "config.toml")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return "", fmt.Errorf("%s is a directory", candidate)
			}
			return candidate, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if isGitRoot(dir) || samePath(dir, home) {
			return "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func isGitRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func samePath(a, b string) bool {
	return b != "" && filepath.Clean(a) == filepath.Clean(b)
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
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
	setString("PAW_BRAIN_PROVIDER", &cfg.Brain.Provider)
	setString("PAW_BRAIN_MODEL", &cfg.Brain.Model)
	setString("PAW_DRONE_TRANSPORT", &cfg.Drone.Transport)
	setString("PAW_DRONE_BASE_URL", &cfg.Drone.BaseURL)
	setString("PAW_DRONE_API_KEY", &cfg.Drone.APIKey)
	setString("PAW_DRONE_PROVIDER", &cfg.Drone.Provider)
	setString("PAW_DRONE_MODEL", &cfg.Drone.Model)
	setString("PAW_TLS_CA_FILE", &cfg.TLS.CAFile)
	if err := setBool("PAW_INSECURE_SKIP_TLS_VERIFY", &cfg.TLS.InsecureSkipVerify); err != nil {
		return err
	}
	if err := setInt("PAW_MAX_TURNS", &cfg.MaxTurns); err != nil {
		return err
	}
	if err := setInt("PAW_MAX_BRAIN_TOKENS", &cfg.MaxBrainTokens); err != nil {
		return err
	}
	if err := setDuration("PAW_CALL_TIMEOUT", &cfg.CallTimeout); err != nil {
		return err
	}
	if err := setBool("PAW_OLLAMA_AUTO_PULL", &cfg.OllamaAutoPull); err != nil {
		return err
	}
	if err := setInt("PAW_GATHER_MAX_DEPTH", &cfg.Gather.MaxDepth); err != nil {
		return err
	}
	if err := setInt("PAW_GATHER_MAX_FILE_BYTES", &cfg.Gather.MaxFileBytes); err != nil {
		return err
	}
	setString("PAW_VERIFY_CMD", &cfg.Verify.Command)
	return setDuration("PAW_VERIFY_TIMEOUT", &cfg.Verify.Timeout)
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

func setBool(key string, dst *bool) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	*dst = b
	return nil
}
