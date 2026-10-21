// Package textutil validates user supplied plain text.
package textutil

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Clean normalises s and reports whether it is acceptable plain text of at
// most maxRunes characters. Control characters are rejected (newlines and tabs
// only when multiline). Text is stored as-is otherwise; HTML is never
// interpreted because every renderer escapes it.
func Clean(s string, maxRunes int, multiline bool) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxRunes {
		return "", false
	}
	for _, r := range s {
		if r == '\n' || r == '\t' {
			if !multiline {
				return "", false
			}
			continue
		}
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return "", false
		}
	}
	return s, true
}
