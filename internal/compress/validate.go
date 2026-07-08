package compress

import (
	"encoding/json"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/schema"
)

const (
	dropUnknownUnit    = "unknown_unit"
	dropPathMismatch   = "path_mismatch"
	dropQuoteMissing   = "quote_missing"
	dropLineOutOfRange = "line_out_of_range"
	dropSchemaError    = "schema_error"
	dropTooManyDropped = "too_many_dropped"
)

type validationStats struct {
	Total   int
	Dropped int
	Reasons map[string]int
}

func validateDigest(raw *envelope.RawContext, digest envelope.ContextDigest) (envelope.ContextDigest, validationStats, error) {
	encoded, err := json.Marshal(digest)
	if err != nil {
		stats := newValidationStats(len(digest.Items))
		stats.drop(dropSchemaError)
		return envelope.ContextDigest{}, stats, err
	}
	if err := schema.ValidateContextDigest(encoded); err != nil {
		stats := newValidationStats(len(digest.Items))
		stats.drop(dropSchemaError)
		return envelope.ContextDigest{}, stats, err
	}
	stats := newValidationStats(len(digest.Items))
	units := rawUnits(raw)
	out := envelope.ContextDigest{Summary: digest.Summary}
	for _, item := range digest.Items {
		unit, ok := units[item.UnitID]
		if !ok {
			stats.drop(dropUnknownUnit)
			continue
		}
		if reason := invalidItemReason(unit, item); reason != "" {
			stats.drop(reason)
			continue
		}
		out.Items = append(out.Items, item)
	}
	return out, stats, nil
}

func newValidationStats(total int) validationStats {
	return validationStats{Total: total, Reasons: map[string]int{}}
}

func (s *validationStats) drop(reason string) {
	s.Dropped++
	s.Reasons[reason]++
}

func rawUnits(raw *envelope.RawContext) map[string]envelope.RawUnit {
	units := map[string]envelope.RawUnit{}
	if raw == nil {
		return units
	}
	for _, unit := range raw.Units {
		units[unit.ID] = unit
	}
	return units
}

func invalidItemReason(unit envelope.RawUnit, item envelope.DigestItem) string {
	if item.Path != unit.Path {
		return dropPathMismatch
	}
	for _, span := range item.Spans {
		if !strings.Contains(unit.Text, span.Quote) {
			return dropQuoteMissing
		}
		if unit.StartLine > 0 || unit.EndLine > 0 {
			if span.StartLine < unit.StartLine || span.EndLine > unit.EndLine || span.StartLine > span.EndLine {
				return dropLineOutOfRange
			}
		}
	}
	return ""
}
