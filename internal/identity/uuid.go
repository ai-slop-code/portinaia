// Package identity owns the canonical resource identity rules used on both sides
// of the discovery protocol. New IDs are UUIDv7; stored UUIDv4 IDs remain valid.
package identity

import (
	"strings"

	"github.com/google/uuid"
)

func New() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func Valid(value string) bool {
	if len(value) != 36 || value != strings.ToLower(value) {
		return false
	}
	id, err := uuid.Parse(value)
	return err == nil && id.String() == value && id.Variant() == uuid.RFC4122 && (id.Version() == 4 || id.Version() == 7)
}
