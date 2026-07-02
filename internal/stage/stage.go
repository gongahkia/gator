package stage

import (
	"context"

	"github.com/gongahkia/paw/internal/envelope"
)

type Stage interface {
	Name() string
	Run(ctx context.Context, in *envelope.Envelope) (*envelope.Envelope, error)
}
