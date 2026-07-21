package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/paw/internal/compress"
	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/egress"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/gather"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/policy"
)

type Runner interface {
	Gather(context.Context, string, string) (*envelope.Envelope, error)
	Compress(context.Context, *envelope.Envelope) (*envelope.Envelope, error)
	Digest(context.Context, string, string) (*envelope.Envelope, error)
}

type StageRunner struct {
	Config          config.Config
	DisableCompress bool
}

func (r StageRunner) Gather(ctx context.Context, cwd, instruction string) (*envelope.Envelope, error) {
	env := envelope.NewEnvelope(taskID(instruction, cwd), instruction, cwd)
	return gather.New(r.Config.Gather).Run(ctx, env)
}

func (r StageRunner) Compress(ctx context.Context, env *envelope.Envelope) (*envelope.Envelope, error) {
	prepared, _, err := egress.Prepare(r.Config.Policy.Egress, env.Raw)
	if err != nil {
		return nil, err
	}
	in := *env
	in.Raw = prepared.Raw
	var drone llm.Client
	if !r.DisableCompress {
		if err := policy.CheckEndpoint(r.Config.Policy, r.Config.Drone.Transport, r.Config.Drone.BaseURL); err != nil {
			return nil, fmt.Errorf("drone endpoint: %w", err)
		}
		drone, err = llm.NewDroneClient(llm.FactoryConfig{Drone: llm.EndpointConfig{
			Transport: r.Config.Drone.Transport,
			BaseURL:   r.Config.Drone.BaseURL,
			APIKey:    r.Config.Drone.APIKey,
			Provider:  r.Config.Drone.Provider,
			Model:     r.Config.Drone.Model,
		}, TLS: llm.TLSConfig{
			CAFile:             r.Config.TLS.CAFile,
			InsecureSkipVerify: r.Config.TLS.InsecureSkipVerify,
		}, CallTimeout: r.Config.CallTimeout})
		if err != nil {
			return nil, err
		}
	}
	stage := compress.New(drone)
	stage.DisableCompress = r.DisableCompress
	return stage.Run(ctx, &in)
}

func (r StageRunner) Digest(ctx context.Context, cwd, instruction string) (*envelope.Envelope, error) {
	env, err := r.Gather(ctx, cwd, instruction)
	if err != nil {
		return nil, err
	}
	return r.Compress(ctx, env)
}

func validateCwdInstruction(cwd, instruction string) error {
	if strings.TrimSpace(cwd) == "" {
		return fmt.Errorf("missing cwd")
	}
	if strings.TrimSpace(instruction) == "" {
		return fmt.Errorf("missing instruction")
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return fmt.Errorf("invalid cwd: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("invalid cwd: not a directory")
	}
	return nil
}

func taskID(instruction, cwd string) string {
	clean := filepath.Clean(cwd)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s", instruction, clean)))
	return "task-" + hex.EncodeToString(sum[:])[:12]
}
