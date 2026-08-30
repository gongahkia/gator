package catalog

import "fmt"

// Apply overlays each patch onto its matching entry while preserving order.
func Apply(entries []Entry, patches []Patch) ([]Entry, error) {
	result := append([]Entry(nil), entries...)
	index := make(map[string]int, len(result))
	for position, entry := range result {
		index[entry.ID] = position
	}
	for _, patch := range patches {
		position, found := index[patch.ID]
		if !found {
			return nil, fmt.Errorf("unknown catalog entry %q", patch.ID)
		}
		if patch.Name != nil {
			result[position].Name = *patch.Name
		}
	}
	return result, nil
}
