package acp

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"time"

	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
)

const (
	protocolVersion = 1
	maxFrameBytes   = 1024 * 1024
	maxPromptBytes  = 512 * 1024
)

// Config supplies the local Gator instance exposed to an ACP client.
// RepositoryPath is intentionally fixed at process start: accepting an
// arbitrary client cwd would let an editor widen the launched agent's scope.
type Config struct {
	Input               io.Reader
	Output              io.Writer
	RepositoryPath      string
	StateDir            string
	DefaultProvider     string
	DefaultModel        string
	DefaultBaseURL      string
	DefaultVerification [][]string
	AgentVersion        string
	ResolveProvider     func(provider, model string) (string, string, error)
	NewExecutor         func(provider, model, baseURL string) (gatorrun.Executor, error)
}

// Server accepts one JSON-RPC message per stdio line. A prompt executes in a
// separate goroutine so cancel requests and responses to permission requests
// remain responsive while a model is running.
type Server struct {
	config     Config
	repository string

	write sync.Mutex
	mu    sync.Mutex

	initialized bool
	sessions    map[string]*session
	permissions map[string]chan tools.CommandDecision
	next        atomic.Uint64
	wait        sync.WaitGroup
}

type session struct {
	id           string
	cwd          string
	provider     string
	model        string
	verification [][]string
	mode         gatorrun.Mode
	title        string
	statePath    string
	updatedAt    time.Time
	active       *activePrompt
	closing      bool
}

type activePrompt struct {
	cancel    context.CancelFunc
	messageID string
	done      chan struct{}
}

type inbound struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type outbound struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
