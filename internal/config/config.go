package config

import "time"

type EndpointConfig struct {
	Transport string
	BaseURL   string
	APIKey    string
	Model     string
}

type GatherConfig struct {
	MaxDepth     int
	MaxFileBytes int
}

type Config struct {
	Brain          EndpointConfig
	Drone          EndpointConfig
	MaxTurns       int
	MaxBrainTokens int
	CallTimeout    time.Duration
	Gather         GatherConfig
}
