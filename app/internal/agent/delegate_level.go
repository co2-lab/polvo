package agent

import "context"

// delegateLevelKey is the unexported context key for the delegate level.
type delegateLevelKey struct{}

// WithDelegateLevel returns a new context carrying the given delegate level.
// Level 0 = main agent, Level 1 = subagent, Level ≥ 2 = sub-subagent (forbidden).
func WithDelegateLevel(ctx context.Context, level int) context.Context {
	return context.WithValue(ctx, delegateLevelKey{}, level)
}

// DelegateLevelFromCtx returns the delegate level stored in the context.
// Returns 0 if no level is set (i.e., main agent context).
func DelegateLevelFromCtx(ctx context.Context) int {
	v, _ := ctx.Value(delegateLevelKey{}).(int)
	return v
}
