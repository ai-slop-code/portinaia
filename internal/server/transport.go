package server

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	apigen "portinaia/internal/api/generated"
)

type problemResponse struct {
	Type      string             `json:"type"`
	Title     string             `json:"title"`
	Status    int                `json:"status"`
	Detail    string             `json:"detail,omitempty"`
	Code      string             `json:"code"`
	RequestID string             `json:"request_id"`
	Readiness *readinessResponse `json:"readiness,omitempty"`
}

const maxCursorLength = 2048

func validCursorLength(value string) bool { return len(value) > 0 && len(value) <= maxCursorLength }

type apiResponseWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *apiResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	if status < 400 {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *apiResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
		w.ResponseWriter.WriteHeader(w.status)
	}
	if w.status >= 400 {
		return w.body.Write(body)
	}
	return w.ResponseWriter.Write(body)
}

func problemMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := &apiResponseWriter{ResponseWriter: w}
		next.ServeHTTP(wrapped, r)
		if wrapped.status < 400 {
			return
		}
		if wrapped.Header().Get("Content-Type") == "application/problem+json" {
			wrapped.ResponseWriter.WriteHeader(wrapped.status)
			_, _ = wrapped.ResponseWriter.Write(wrapped.body.Bytes())
			return
		}
		wrapped.Header().Set("Content-Type", "application/problem+json")
		wrapped.Header().Set("Cache-Control", "no-store")
		wrapped.Header().Del("Content-Length")
		detail := strings.TrimSpace(wrapped.body.String())
		if wrapped.status >= 500 {
			detail = "internal server error"
		}
		requestID := requestIdentity(r.Context())
		if requestID == "" {
			requestID = r.Header.Get("X-Request-ID")
		}
		if !validRequestID(requestID) {
			requestID, _ = randomID()
			requestID = "req_" + requestID
		}
		wrapped.ResponseWriter.WriteHeader(wrapped.status)
		_ = json.NewEncoder(wrapped.ResponseWriter).Encode(problemResponse{Type: "about:blank", Title: http.StatusText(wrapped.status), Status: wrapped.status, Detail: fmt.Sprintf("%.*s", 2000, detail), Code: fmt.Sprintf("http_%d", wrapped.status), RequestID: requestID})
	})
}

func writeProblemForRequest(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	requestID := requestIdentity(r.Context())
	if requestID == "" {
		requestID = r.Header.Get("X-Request-ID")
	}
	if !validRequestID(requestID) {
		requestID, _ = randomID()
		requestID = "req_" + requestID
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("Content-Length")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problemResponse{Type: "about:blank", Title: http.StatusText(status), Status: status, Detail: fmt.Sprintf("%.*s", 2000, detail), Code: code, RequestID: requestID})
}

// decodeJSON applies a strict, bounded JSON boundary to request payloads.
func decodeJSON(w http.ResponseWriter, r *http.Request, limit int64, target any) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		return err
	}
	return decodeStrictRequest(body, target)
}

func decodeStrictRequest(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request must contain exactly one JSON value")
		}
		return err
	}
	return rejectExplicitNulls(reflect.TypeOf(target), json.RawMessage(body))
}

func writeJSONDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) {
		writeProblemForRequest(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds the allowed size")
		return
	}
	writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", "request JSON is invalid")
}

func rejectExplicitNulls(targetType reflect.Type, raw json.RawMessage) error {
	if targetType == nil {
		return nil
	}
	if targetType.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return rejectExplicitNulls(targetType.Elem(), raw)
	}
	if targetType == reflect.TypeOf(json.RawMessage{}) || targetType.Kind() == reflect.Interface {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("explicit null is not allowed for this field")
	}
	jsonUnmarshaler := reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	if targetType.Implements(jsonUnmarshaler) || (targetType.Kind() != reflect.Pointer && reflect.PointerTo(targetType).Implements(jsonUnmarshaler)) {
		return nil
	}
	textUnmarshaler := reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	if targetType.Implements(textUnmarshaler) || (targetType.Kind() != reflect.Pointer && reflect.PointerTo(targetType).Implements(textUnmarshaler)) {
		return nil
	}
	switch targetType.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return err
		}
		for index := 0; index < targetType.NumField(); index++ {
			field := targetType.Field(index)
			if !field.IsExported() {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			if value, exists := object[name]; exists {
				if bytes.Equal(bytes.TrimSpace(value), []byte("null")) && field.Type.Kind() == reflect.Pointer && !nullableJSONField(targetType, field) {
					return fmt.Errorf("%s: explicit null is not allowed", name)
				}
				if err := rejectExplicitNulls(field.Type, value); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
			}
		}
	case reflect.Slice, reflect.Array:
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return err
		}
		for _, value := range values {
			if err := rejectExplicitNulls(targetType.Elem(), value); err != nil {
				return err
			}
		}
	case reflect.Map:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return err
		}
		for _, value := range object {
			if err := rejectExplicitNulls(targetType.Elem(), value); err != nil {
				return err
			}
		}
	}
	return nil
}

func nullableJSONField(parent reflect.Type, field reflect.StructField) bool {
	if field.Tag.Get("nullable") == "true" {
		return true
	}
	if parent == reflect.TypeOf(apigen.ProviderWrite{}) {
		return field.Name == "Website" || field.Name == "ExternalAccountReference"
	}
	if parent == reflect.TypeOf(apigen.ComputeWrite{}) {
		return field.Name == "ProviderResourceId"
	}
	if parent == reflect.TypeOf(apigen.DomainWrite{}) {
		return field.Name == "AutoRenew"
	}
	if parent == reflect.TypeOf(apigen.IpAssignmentWrite{}) {
		return field.Name == "PrefixLength"
	}
	if parent == reflect.TypeOf(apigen.AgentHeartbeatRequest{}) {
		return field.Name == "LastCollectionError"
	}
	return false
}
