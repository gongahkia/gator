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
	units := verifyFailureUnits(in)
	listing, err := collectDirListing(in.Cwd, g.Config.MaxDepth, g.Config.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	units = append(units, listing...)
	symbols, err := collectSymbols(ctx, in.Cwd, g.Config.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	units = append(units, symbols...)
	hits, err := collectSearchHits(ctx, in.Cwd, in.Instruction, g.Config.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	units = append(units, hits...)
	slices, err := collectFileSlices(in.Cwd, hits, g.Config.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	units = append(units, slices...)
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

func verifyFailureUnits(in *envelope.Envelope) []envelope.RawUnit {
	if in.Verify == nil || in.Verify.FailureDigest == "" {
		return nil
	}
	return []envelope.RawUnit{{
		Kind: "verify_failure",
		Text: in.Verify.FailureDigest,
	}}
}
