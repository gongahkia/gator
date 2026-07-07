package pricing

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	BrainInput         = "brain:in"
	BrainOutput        = "brain:out"
	BrainCacheCreation = "brain:cache_creation"
	BrainCacheRead     = "brain:cache_read"
	DroneTotal         = "drone"
)

type Rates map[string]float64

func ParseRates(raw string) (Rates, error) {
	rates := Rates{}
	if strings.TrimSpace(raw) == "" {
		return rates, nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("rate %q must be key=value", part)
		}
		key, err := normalizeKey(strings.TrimSpace(key))
		if err != nil {
			return nil, err
		}
		rate, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil, fmt.Errorf("rate %q: %w", part, err)
		}
		if rate < 0 {
			return nil, fmt.Errorf("rate %q must be non-negative", part)
		}
		rates[key] = rate
	}
	return rates, nil
}

func (r Rates) Cost(key string, tokens int) (float64, bool) {
	rate, ok := r[key]
	if !ok {
		return 0, false
	}
	return float64(tokens) * rate / 1_000_000, true
}

func (r Rates) BrainCost(input, output, cacheCreation, cacheRead int) (float64, bool) {
	total := 0.0
	ok := false
	for _, item := range []struct {
		key    string
		tokens int
	}{
		{BrainInput, input},
		{BrainOutput, output},
		{BrainCacheCreation, cacheCreation},
		{BrainCacheRead, cacheRead},
	} {
		cost, found := r.Cost(item.key, item.tokens)
		if found {
			total += cost
			ok = true
		}
	}
	return total, ok
}

func normalizeKey(key string) (string, error) {
	switch strings.ToLower(key) {
	case BrainInput, "brain:input":
		return BrainInput, nil
	case BrainOutput, "brain:output":
		return BrainOutput, nil
	case BrainCacheCreation, "brain:cache-creation":
		return BrainCacheCreation, nil
	case BrainCacheRead, "brain:cache-read":
		return BrainCacheRead, nil
	case DroneTotal, "drone:total":
		return DroneTotal, nil
	default:
		return "", fmt.Errorf("unknown rate key %q", key)
	}
}
