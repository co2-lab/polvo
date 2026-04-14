package tool

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/co2-lab/polvo/internal/secrets"
)

// secretsMaskingTool wraps a Tool and masks secrets in all Execute results.
type secretsMaskingTool struct {
	inner Tool
}

// SecretsMaskingMiddleware returns a Tool that wraps inner and masks secrets
// in all execution results before returning them to the caller.
func SecretsMaskingMiddleware(inner Tool) Tool {
	return &secretsMaskingTool{inner: inner}
}

func (s *secretsMaskingTool) Name() string                 { return s.inner.Name() }
func (s *secretsMaskingTool) Description() string          { return s.inner.Description() }
func (s *secretsMaskingTool) InputSchema() json.RawMessage { return s.inner.InputSchema() }

func (s *secretsMaskingTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	result, err := s.inner.Execute(ctx, input)
	if result == nil {
		return result, err
	}

	masked := secrets.MaskSecretsDetailed(result.Content)
	if len(masked.Redactions) > 0 {
		patterns := make([]string, len(masked.Redactions))
		for i, r := range masked.Redactions {
			patterns[i] = r.Pattern
		}
		slog.Warn("secrets_masked",
			"tool", s.inner.Name(),
			"count", len(masked.Redactions),
			"patterns", patterns,
		)
	}

	return &Result{
		Content: masked.Masked,
		IsError: result.IsError,
	}, err
}
