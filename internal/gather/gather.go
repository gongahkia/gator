package gather

import (
	"context"
	"fmt"

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

func (g *Gather) Run(ctx context.Context, in *envelope.Envelope) (*envelope.Envelope, error) {
	out := *in
	units, err := collectSearchHits(ctx, in.Cwd, in.Instruction, g.Config.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	assignIDs(units)
	out.Stage = g.Name()
	out.Raw = &envelope.RawContext{Units: units, TotalBytes: totalBytes(units)}
	return &out, nil
}

func assignIDs(units []envelope.RawUnit) {
	for i := range units {
		units[i].ID = fmt.Sprintf("u%03d", i+1)
	}
}

func totalBytes(units []envelope.RawUnit) int {
	total := 0
	for _, unit := range units {
		total += len(unit.Text)
	}
	return total
}
