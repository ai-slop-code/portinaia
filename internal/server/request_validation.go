package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

// decodeOperationJSON is the shared raw-boundary primitive for generated request
// DTOs. Call it before any handler-side effects; zero values remain valid when present.
func decodeOperationJSON(w http.ResponseWriter, r *http.Request, limit int64, target any, required []string, bounds map[string]int) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		return err
	}
	if err = validateRequiredMembers(body, target, required...); err != nil {
		return err
	}
	if err = decodeStrictRequest(body, target); err != nil {
		return err
	}
	return validateBounds(target, bounds)
}

// validateRequiredMembers checks raw object membership before decoding can erase
// the distinction between an omitted field and its Go zero value.
func validateRequiredMembers(body []byte, target any, required ...string) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil {
		return err
	}
	if members == nil {
		return fmt.Errorf("request body must be a JSON object")
	}
	for _, name := range required {
		raw, ok := members[name]
		if !ok {
			return fmt.Errorf("%s: required member is missing", name)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s: required member cannot be null", name)
		}
	}
	return nil
}

// validateBounds is a narrow post-decode guard for fields with contract limits.
// Operation-specific validators should call it after strict decoding.
func validateBounds(value any, limits map[string]int) error {
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct request")
	}
	for index := 0; index < rv.NumField(); index++ {
		field := rv.Type().Field(index)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		limit, constrained := limits[name]
		if !constrained {
			continue
		}
		current := rv.Field(index)
		if current.Kind() == reflect.Pointer {
			if current.IsNil() {
				continue
			}
			current = current.Elem()
		}
		size := -1
		switch current.Kind() {
		case reflect.String:
			size = len(current.String())
		case reflect.Slice, reflect.Array:
			size = current.Len()
		}
		if size > limit {
			return fmt.Errorf("%s: exceeds maximum of %d", name, limit)
		}
	}
	return nil
}
