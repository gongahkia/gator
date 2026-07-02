package gather

import (
	"context"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
)

type Gather struct {
	Config config.GatherConfig
}

func New(cfg config.GatherConfig) *Gather {
	return &Gather{Config: cfg}
}

func (g *Gather) Name() string {
	return "gather"
}

func (g *Gather) Run(_ context.Context, in *envelope.Envelope) (*envelope.Envelope, error) {
	out := *in
	out.Stage = g.Name()
	out.Raw = &envelope.RawContext{}
	return &out, nil
}
