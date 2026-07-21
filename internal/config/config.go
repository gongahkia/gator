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

type VerifyConfig struct {
	Command string        `toml:"command"`
	Timeout time.Duration `toml:"timeout"`
}

type ProviderPolicy struct {
	AllowedTransports []string `toml:"allowed_transports"`
	AllowedBaseURLs   []string `toml:"allowed_base_urls"`
	AllowLoopback     bool     `toml:"allow_loopback"`
}

type CommandPolicy struct {
	Allow           []string `toml:"allow"`
	Deny            []string `toml:"deny"`
	RequireApproval bool     `toml:"require_approval"`
}

type RiskPolicy struct {
	MaxFiles    int `toml:"max_files"`
	MaxLines    int `toml:"max_lines"`
	MaxTokens   int `toml:"max_tokens"`
	MaxCommands int `toml:"max_commands"`
}

type GitPolicy struct {
	AllowedRemotes []string `toml:"allowed_remotes"`
	AllowBranch    bool     `toml:"allow_branch"`
	AllowCommit    bool     `toml:"allow_commit"`
	AllowPush      bool     `toml:"allow_push"`
}

type EgressPolicy struct {
	MaxFiles     int  `toml:"max_files"`
	MaxBytes     int  `toml:"max_bytes"`
	BlockSecrets bool `toml:"block_secrets"`
}

type ApprovalPolicy struct {
	AutoApprove bool `toml:"auto_approve"`
}

type PolicyConfig struct {
	Version  string         `toml:"version"`
	Provider ProviderPolicy `toml:"provider"`
	Command  CommandPolicy  `toml:"command"`
	Risk     RiskPolicy     `toml:"risk"`
	Git      GitPolicy      `toml:"git"`
	Egress   EgressPolicy   `toml:"egress"`
	Approval ApprovalPolicy `toml:"approval"`
}

type Config struct {
	Brain          EndpointConfig `toml:"brain"`
	Drone          EndpointConfig `toml:"drone"`
	TLS            TLSConfig      `toml:"tls"`
	Verify         VerifyConfig   `toml:"verify"`
	MaxTurns       int            `toml:"max_turns"`
	MaxBrainTokens int            `toml:"max_brain_tokens"`
	CallTimeout    time.Duration  `toml:"call_timeout"`
	OllamaAutoPull bool           `toml:"ollama_auto_pull"`
	Gather         GatherConfig   `toml:"gather"`
	Policy         PolicyConfig   `toml:"policy"`
}
