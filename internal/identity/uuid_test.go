package identity

import (
	"strings"
	"testing"
)

func TestCanonicalResourceIdentities(t *testing.T) {
	id, err := New()
	if err != nil || !Valid(id) || id[14] != '7' {
		t.Fatalf("new identity = %q: %v", id, err)
	}
	if !Valid("f0000000-0000-4000-8000-000000000001") {
		t.Fatal("legacy v4 rejected")
	}
	for _, value := range []string{
		"f0000000-0000-1000-8000-000000000001", // unsupported version
		"f0000000-0000-4000-c000-000000000001", // unsupported variant
		"F0000000-0000-4000-8000-000000000001",
		strings.ReplaceAll(id, "-", ""), "urn:uuid:" + id, "{" + id + "}",
	} {
		if Valid(value) {
			t.Errorf("accepted noncanonical identity %q", value)
		}
	}
}
