package pricing

import (
	"math"
	"testing"
)

func TestParseRatesAndCost(t *testing.T) {
	rates, err := ParseRates("brain:in=3.0,brain:out=15,brain:cache_creation=4,drone=0.25")
	if err != nil {
		t.Fatalf("parse rates: %v", err)
	}
	brain, ok := rates.BrainCost(1000, 200, 300, 700)
	if !ok {
		t.Fatal("missing brain cost")
	}
	if math.Abs(brain-0.0072) > 0.00000001 {
		t.Fatalf("brain cost = %.8f", brain)
	}
	drone, ok := rates.Cost(DroneTotal, 400)
	if !ok || math.Abs(drone-0.0001) > 0.00000001 {
		t.Fatalf("drone cost = %.8f ok=%t", drone, ok)
	}
}

func TestParseRatesRejectsUnknownKey(t *testing.T) {
	if _, err := ParseRates("brain:weird=1"); err == nil {
		t.Fatal("expected unknown key error")
	}
}
