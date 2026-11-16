package server

import "testing"

func TestRandomIDUsesUUIDv7(t *testing.T) {
	id, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 || id[14] != '7' || (id[19] != '8' && id[19] != '9' && id[19] != 'a' && id[19] != 'b') {
		t.Fatalf("randomID() = %q, want canonical UUIDv7", id)
	}
}
