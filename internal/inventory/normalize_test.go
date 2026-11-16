package inventory

import "testing"

func TestNormalizeDNSName(t *testing.T) {
	got, err := NormalizeDNSName(" BÜCHER.example. ")
	if err != nil || got != "xn--bcher-kva.example" {
		t.Fatalf("NormalizeDNSName() = %q, %v", got, err)
	}
	for _, value := range []string{"*.example", "bad..example", "-bad.example", ""} {
		if _, err := NormalizeDNSName(value); err == nil {
			t.Errorf("NormalizeDNSName(%q) unexpectedly succeeded", value)
		}
	}
}

func TestCanonicalIP(t *testing.T) {
	for _, test := range []struct {
		input, want string
		family      int
	}{{"192.0.2.1", "192.0.2.1", 4}, {"2001:0db8::1", "2001:db8::1", 6}} {
		got, family, err := CanonicalIP(test.input)
		if err != nil || got != test.want || family != test.family {
			t.Errorf("CanonicalIP(%q) = %q, %d, %v", test.input, got, family, err)
		}
	}
	for _, value := range []string{"192.0.2.1/24", "fe80::1%en0", "not-an-ip"} {
		if _, _, err := CanonicalIP(value); err == nil {
			t.Errorf("CanonicalIP(%q) unexpectedly succeeded", value)
		}
	}
}
