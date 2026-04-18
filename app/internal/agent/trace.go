package agent

import (
	"context"

	"github.com/co2-lab/polvo/internal/provider"
)

// traceChat is a passthrough — instrumentation is handled via slog in the loop.
func traceChat(ctx context.Context, _ string, fn func(context.Context) (*provider.ChatResponse, error)) (*provider.ChatResponse, error) {
	return fn(ctx)
}
