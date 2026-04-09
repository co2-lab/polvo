package template

import (
	"bytes"
	"fmt"
	"text/template"
)

// Render renders a prompt template with the given data.
func Render(tmplContent string, data *Data) (string, error) {
	tmpl, err := template.New("prompt").Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}
