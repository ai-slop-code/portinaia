package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestValidateRequiredMembersDistinguishesMissingNullAndZero(t *testing.T) {
	for _, body := range []string{`{"name":"x"}`, `{"name":"x","primary":null}`} {
		if err := validateRequiredMembers([]byte(body), nil, "primary"); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	if err := validateRequiredMembers([]byte(`{"name":"x","primary":false}`), nil, "primary"); err != nil {
		t.Fatalf("valid zero value rejected: %v", err)
	}
}

func TestDecodeOperationJSONRequiresRawMembersButAcceptsZero(t *testing.T) {
	type input struct {
		Enabled bool   `json:"enabled"`
		Name    string `json:"name"`
	}
	for _, body := range []string{`{"name":"x"}`, `{"name":"x","enabled":null}`} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		var value input
		if err := decodeOperationJSON(httptest.NewRecorder(), r, 1024, &value, []string{"enabled"}, nil); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"enabled":false,"name":"x"}`))
	var value input
	if err := decodeOperationJSON(httptest.NewRecorder(), r, 1024, &value, []string{"enabled"}, map[string]int{"name": 2}); err != nil {
		t.Fatalf("valid zero rejected: %v", err)
	}
}

func TestValidateBounds(t *testing.T) {
	type request struct {
		Name  string `json:"name"`
		Items []int  `json:"items"`
	}
	if err := validateBounds(request{Name: "four", Items: []int{1, 2}}, map[string]int{"name": 3}); err == nil {
		t.Fatal("accepted oversized string")
	}
	if err := validateBounds(request{Name: "ok", Items: []int{1, 2}}, map[string]int{"items": 1}); err == nil {
		t.Fatal("accepted oversized array")
	}
}

func TestPathIDValidationRunsBeforeHandler(t *testing.T) {
	called := false
	var route chi.Context
	route.URLParams.Add("provider_id", "not-a-uuid")
	req := httptest.NewRequest(http.MethodPut, "/api/v1/providers/not-a-uuid", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &route))
	response := httptest.NewRecorder()
	uuidPathMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(response, req)
	if called {
		t.Fatal("handler ran for malformed resource ID")
	}
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestRequestIdentityValidatesAndPropagatesCorrelationID(t *testing.T) {
	for _, tc := range []struct {
		input string
		valid bool
	}{{"req-123", true}, {"bad id", false}, {string(make([]byte, 129)), false}} {
		if validRequestID(tc.input) != tc.valid {
			t.Errorf("validRequestID(%q) = %v, want %v", tc.input, validRequestID(tc.input), tc.valid)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	req.Header.Set("X-Request-ID", "trace-7")
	response := httptest.NewRecorder()
	requestIdentityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestIdentity(r.Context()); got != "trace-7" {
			t.Errorf("context request ID = %q", got)
		}
	})).ServeHTTP(response, req)
	if got := response.Header().Get("X-Request-ID"); got != "trace-7" {
		t.Fatalf("response request ID = %q", got)
	}
}
