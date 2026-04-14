package provider

// ModelPricing defines per-token costs in USD for a model.
type ModelPricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

var modelPricingTable = map[string]ModelPricing{
	"claude-opus-4-5":            {15.00, 75.00, 1.50, 18.75},
	"claude-sonnet-4-5":          {3.00, 15.00, 0.30, 3.75},
	"claude-haiku-4-5-20251001":  {0.80, 4.00, 0.08, 1.00},
	"claude-3-5-sonnet-20241022": {3.00, 15.00, 0.30, 3.75},
	"claude-3-5-sonnet-20240620": {3.00, 15.00, 0.30, 3.75},
	"claude-3-opus-20240229":     {15.00, 75.00, 1.50, 18.75},
	"claude-3-haiku-20240307":    {0.25, 1.25, 0.03, 0.30},
	"claude-3-5-haiku-20241022":  {0.80, 4.00, 0.08, 1.00},
	"claude-3-7-sonnet-20250219": {3.00, 15.00, 0.30, 3.75},
}

// ComputeCostUSD returns estimated cost in USD for the given usage and model.
// Returns 0 if model is unknown (never panics).
func ComputeCostUSD(usage TokenUsage, model string) float64 {
	p, ok := modelPricingTable[model]
	if !ok {
		return 0
	}
	return float64(usage.PromptTokens)*p.InputPerMillion/1_000_000 +
		float64(usage.CompletionTokens)*p.OutputPerMillion/1_000_000 +
		float64(usage.CacheReadTokens)*p.CacheReadPerMillion/1_000_000 +
		float64(usage.CacheWriteTokens)*p.CacheWritePerMillion/1_000_000
}
