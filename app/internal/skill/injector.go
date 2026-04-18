package skill

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/co2-lab/polvo/internal/memory"
)

// Injector retrieves the most relevant stored skills for a given task and
// formats them as a markdown block for injection into a system prompt.
type Injector struct {
	Store *memory.Store
	TopK  int
}

// Recall returns a formatted markdown block of relevant skills for taskDescription,
// or an empty string if none are found or an error occurs.
func (inj *Injector) Recall(ctx context.Context, taskDescription string) string {
	topK := inj.TopK
	if topK == 0 {
		topK = 5
	}
	entries, err := inj.Store.SemanticRead(ctx, taskDescription, memory.Filter{
		Type:  "decision",
		Limit: 100,
	}, topK)
	if err != nil || len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Procedimentos conhecidos para este projeto:\n")
	for _, e := range entries {
		ts := time.Unix(0, e.Timestamp).Format("2006-01-02")
		fmt.Fprintf(&sb, "- [%s] %s\n", ts, e.Content)
	}
	return sb.String()
}
