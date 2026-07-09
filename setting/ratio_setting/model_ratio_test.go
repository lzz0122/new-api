package ratio_setting

import (
	"math"
	"testing"
)

func TestGPT56DefaultPricingRatios(t *testing.T) {
	InitRatioSettings()

	models := []string{
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
	}

	for _, model := range models {
		model := model
		t.Run(model, func(t *testing.T) {
			modelRatio, ok, matchName := GetModelRatio(model)
			if !ok {
				t.Fatalf("GetModelRatio(%q) not configured, matched %q", model, matchName)
			}
			if math.Abs(modelRatio-2.5) > 1e-9 {
				t.Fatalf("model ratio = %v, want 2.5", modelRatio)
			}

			completionRatio := GetCompletionRatio(model)
			if math.Abs(completionRatio-6) > 1e-9 {
				t.Fatalf("completion ratio = %v, want 6", completionRatio)
			}

			cacheRatio, ok := GetCacheRatio(model)
			if !ok {
				t.Fatalf("GetCacheRatio(%q) not configured", model)
			}
			if math.Abs(cacheRatio-0.1) > 1e-9 {
				t.Fatalf("cache ratio = %v, want 0.1", cacheRatio)
			}
		})
	}
}
