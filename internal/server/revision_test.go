package server

import "testing"

func TestParseRevisionETagRequiresCanonicalStrongTag(t *testing.T) {
	for value, want := range map[string]int64{
		`"revision-1"`:  1,
		`"revision-42"`: 42,
	} {
		got, ok := parseRevisionETag(value)
		if !ok || got != want {
			t.Errorf("parseRevisionETag(%q) = %d,%v, want %d,true", value, got, ok, want)
		}
	}
	for _, value := range []string{
		"",
		`W/"revision-1"`,
		`"revision-0"`,
		`"revision-01"`,
		`"revision-1x"`,
		`"revision-9223372036854775808"`,
	} {
		if got, ok := parseRevisionETag(value); ok {
			t.Errorf("parseRevisionETag(%q) = %d,true, expected invalid", value, got)
		}
	}
}
