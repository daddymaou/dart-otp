package templates

import (
	"html"
	"regexp"
)

var codePlaceholder = regexp.MustCompile(`\{\{code\}\}`)

// Render HTML-escapes the body then substitutes the code as a monospace block.
func Render(body, code string) string {
	return codePlaceholder.ReplaceAllString(html.EscapeString(body), "<code>"+code+"</code>")
}
