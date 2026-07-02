package compress

import (
	"encoding/json"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/schema"
)

type validationStats struct {
	Total   int
	Dropped int
}

func validateDigest(raw *envelope.RawContext, digest envelope.ContextDigest) (envelope.ContextDigest, validationStats, error) {
	encoded, err := json.Marshal(digest)
	if err != nil {
		return envelope.ContextDigest{}, validationStats{}, err
	}
	if err := schema.ValidateContextDigest(encoded); err != nil {
		return envelope.ContextDigest{}, validationStats{}, err
	}
	stats := validationStats{Total: len(digest.Items)}
	units := rawUnits(raw)
	out := envelope.ContextDigest{Summary: digest.Summary}
	for _, item := range digest.Items {
		unit, ok := units[item.UnitID]
		if !ok || !validItem(unit, item) {
			stats.Dropped++
			continue
		}
		out.Items = append(out.Items, item)
	}
	return out, stats, nil
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

func validItem(unit envelope.RawUnit, item envelope.DigestItem) bool {
	if item.Path != unit.Path {
		return false
	}
	for _, span := range item.Spans {
		if !strings.Contains(unit.Text, span.Quote) {
			return false
		}
		if unit.StartLine > 0 || unit.EndLine > 0 {
			if span.StartLine < unit.StartLine || span.EndLine > unit.EndLine || span.StartLine > span.EndLine {
				return false
			}
		}
	}
	return true
}
