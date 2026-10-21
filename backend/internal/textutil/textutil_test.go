package textutil

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	cases := []struct {
		in        string
		max       int
		multiline bool
		want      string
		ok        bool
	}{
		{"  hello ", 10, false, "hello", true},
		{"a\r\nb", 10, true, "a\nb", true},
		{"a\nb", 10, false, "", false},
		{"tab\there", 20, false, "", false},
		{"null\x00byte", 20, true, "", false},
		{"bell\x07", 20, true, "", false},
		{"line sep", 20, true, "", false},
		{"<b>html</b>", 20, false, "<b>html</b>", true}, // stored as text, escaped on render
		{strings.Repeat("守", 5), 5, false, strings.Repeat("守", 5), true},
		{strings.Repeat("守", 6), 5, false, "", false},
		{"\xff\xfe", 10, false, "", false},
	}
	for _, tc := range cases {
		got, ok := Clean(tc.in, tc.max, tc.multiline)
		if ok != tc.ok || got != tc.want {
			t.Errorf("Clean(%q) = %q,%v want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
