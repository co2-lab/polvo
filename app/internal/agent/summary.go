package agent

import (
	"regexp"
	"strings"
)

// InlineSummarySystemSuffix is appended to the system prompt when no dedicated
// summary model is configured. It instructs the model to include a compact
// summary of its own response inside <summary> tags.
const InlineSummarySystemSuffix = `

After your response, append a one or two sentence summary of what you did or answered, enclosed in <summary> tags on a new line:
<summary>your summary here</summary>`

var summaryTagRe = regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)

// ExtractInlineSummary removes the first <summary>...</summary> block from
// content and returns the extracted summary text and the cleaned content.
// If no tag is found, summary is empty and cleaned equals content unchanged.
func ExtractInlineSummary(content string) (summary, cleaned string) {
	m := summaryTagRe.FindStringSubmatchIndex(content)
	if m == nil {
		return "", content
	}
	summary = strings.TrimSpace(content[m[2]:m[3]])
	cleaned = strings.TrimRight(content[:m[0]], " \t\n") + content[m[1]:]
	cleaned = strings.TrimSpace(cleaned)
	return summary, cleaned
}
