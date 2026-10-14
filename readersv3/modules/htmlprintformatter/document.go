package htmlprintformatter

import (
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
)

// Tokenize only to locate the insertion offset. Never parse/re-serialize the
// supplied source: that would rewrite scripts, attributes and document syntax.
func withBase(source, base string) string {
	if base == "" {
		return source
	}
	z := xhtml.NewTokenizer(strings.NewReader(source))
	offset, insert, fallback := 0, -1, 0
	inTemplate := 0
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break
		}
		rawLen := len(z.Raw())
		t := z.Token()
		if tt == xhtml.EndTagToken && t.Data == "template" {
			inTemplate--
		}
		if tt == xhtml.StartTagToken || tt == xhtml.SelfClosingTagToken {
			if t.Data == "template" {
				inTemplate++
			}
			if inTemplate == 0 {
				if t.Data == "base" {
					return source
				}
				if t.Data == "head" && insert < 0 {
					insert = offset + rawLen
				}
				if t.Data == "html" {
					fallback = offset + rawLen
				}
			}
		}
		if tt == xhtml.DoctypeToken && fallback == 0 {
			fallback = offset + rawLen
		}
		offset += rawLen
	}
	if insert < 0 {
		insert = fallback
	}
	return source[:insert] + `<base href="` + html.EscapeString(base) + `">` + source[insert:]
}
