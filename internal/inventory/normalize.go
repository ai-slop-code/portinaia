// Package inventory contains transport-independent inventory value rules.
package inventory

import (
	"errors"
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
)

// NormalizeDNSName returns a lower-case IDNA ASCII domain without a terminal dot.
func NormalizeDNSName(value string) (string, error) {
	value = strings.TrimSuffix(strings.TrimSpace(value), ".")
	if value == "" || strings.Contains(value, "*") {
		return "", errors.New("invalid dns name")
	}
	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil {
		return "", err
	}
	ascii = strings.ToLower(ascii)
	if len(ascii) > 253 {
		return "", errors.New("dns name too long")
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("invalid dns label")
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return "", errors.New("invalid dns label")
			}
		}
	}
	return ascii, nil
}

// CanonicalIP validates a non-zoned IPv4/IPv6 address and returns its family.
func CanonicalIP(value string) (string, int, error) {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || address.Zone() != "" {
		return "", 0, errors.New("address must be a valid IPv4 or IPv6 address")
	}
	family := 6
	if address.Is4() {
		family = 4
	}
	return address.String(), family, nil
}
