package agent

import (
	"regexp"
	"strings"
)

// summaryDeltaProxy wraps an OnTextDelta callback and suppresses any
// <summary>...</summary> block — including the whitespace that precedes it —
// that arrives mid-stream, forwarding all other content immediately.
//
// State machine:
//
//	normal  → accumulate whitespace into wsBuf; emit wsBuf when non-ws arrives
//	          unless it starts a <summary> tag (enter tagBuf mode)
//	tagBuf  → accumulate into tagBuf while it's a prefix of "<summary>"
//	          confirmed → enter inTag mode (discard wsBuf + tagBuf)
//	          mismatch  → flush wsBuf + tagBuf and return to normal
//	inTag   → discard everything until "</summary>" closes the block
type summaryDeltaProxy struct {
	emit  func(string)
	wsBuf strings.Builder // pending whitespace before a potential <summary>
	tBuf  strings.Builder // accumulates a potential <summary> opener
	inTag bool            // true while inside <summary>…</summary>
}

func newSummaryDeltaProxy(emit func(string)) *summaryDeltaProxy {
	return &summaryDeltaProxy{emit: emit}
}

const summaryOpen  = "<summary>"
const summaryClose = "</summary>"

// Write processes one streaming delta.
func (p *summaryDeltaProxy) Write(delta string) {
	for _, r := range delta {
		p.writeRune(r)
	}
}

func (p *summaryDeltaProxy) writeRune(r rune) {
	ch := string(r)

	if p.inTag {
		p.tBuf.WriteString(ch)
		if strings.HasSuffix(p.tBuf.String(), summaryClose) {
			p.tBuf.Reset()
			p.inTag = false
		}
		return
	}

	if p.tBuf.Len() > 0 {
		// Currently matching a potential <summary> opener.
		p.tBuf.WriteString(ch)
		candidate := p.tBuf.String()
		if strings.HasPrefix(summaryOpen, candidate) {
			if candidate == summaryOpen {
				// Confirmed — discard wsBuf and tagBuf, enter inTag.
				p.wsBuf.Reset()
				p.tBuf.Reset()
				p.inTag = true
			}
			return
		}
		// Mismatch — flush wsBuf + tBuf and resume normal.
		p.emit(p.wsBuf.String() + candidate)
		p.wsBuf.Reset()
		p.tBuf.Reset()
		return
	}

	if r == '<' {
		// Could be start of <summary> — hold whitespace and start tag buffer.
		p.tBuf.WriteString(ch)
		return
	}

	if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
		// Whitespace: hold in wsBuf in case it precedes <summary>.
		p.wsBuf.WriteString(ch)
		return
	}

	// Regular content — flush pending whitespace then emit.
	if p.wsBuf.Len() > 0 {
		p.emit(p.wsBuf.String())
		p.wsBuf.Reset()
	}
	p.emit(ch)
}

// Flush forwards any buffered content that never resolved into a tag.
func (p *summaryDeltaProxy) Flush() {
	if p.inTag {
		return // incomplete tag — discard
	}
	pending := p.wsBuf.String() + p.tBuf.String()
	if pending != "" {
		p.emit(pending)
	}
	p.wsBuf.Reset()
	p.tBuf.Reset()
}

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
