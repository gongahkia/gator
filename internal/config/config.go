package config

import "time"

type EndpointConfig struct {
	Transport string `toml:"transport"`
	BaseURL   string `toml:"base_url"`
	APIKey    string `toml:"api_key"`
	Provider  string `toml:"provider"`
	Model     string `toml:"model"`
}

type GatherConfig struct {
	MaxDepth     int `toml:"max_depth"`
	MaxFileBytes int `toml:"max_file_bytes"`
}

type TLSConfig struct {
	CAFile             string `toml:"ca_file"`
	InsecureSkipVerify bool   `toml:"insecure_skip_verify"`
}

type Config struct {
	Brain          EndpointConfig `toml:"brain"`
	Drone          EndpointConfig `toml:"drone"`
	TLS            TLSConfig      `toml:"tls"`
	MaxTurns       int            `toml:"max_turns"`
	MaxBrainTokens int            `toml:"max_brain_tokens"`
	CallTimeout    time.Duration  `toml:"call_timeout"`
	OllamaAutoPull bool           `toml:"ollama_auto_pull"`
	Gather         GatherConfig   `toml:"gather"`
}
