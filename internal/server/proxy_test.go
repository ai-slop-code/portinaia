package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyClientIPResolution(t *testing.T) {
	cases := []struct {
		name      string
		trusted   []string
		remote    string
		forwarded string
		want      string
	}{
		{name: "untrusted forwarded header ignored", remote: "198.51.100.10:8080", forwarded: "203.0.113.5", want: "198.51.100.10"},
		{name: "trusted immediate proxy", trusted: []string{"192.0.2.0/24"}, remote: "192.0.2.10:8080", forwarded: "203.0.113.5", want: "203.0.113.5"},
		{name: "trusted proxy chain", trusted: []string{"192.0.2.0/24"}, remote: "192.0.2.10:8080", forwarded: "203.0.113.5, 192.0.2.11", want: "203.0.113.5"},
		{name: "malformed trusted chain ignored", trusted: []string{"192.0.2.0/24"}, remote: "192.0.2.10:8080", forwarded: "not-an-ip", want: "192.0.2.10"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var got string
			handler := trustedProxyMiddleware(test.trusted)(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				got = ClientIP(request).String()
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remote
			request.Header.Set("X-Forwarded-For", test.forwarded)
			handler.ServeHTTP(httptest.NewRecorder(), request)
			if got != test.want {
				t.Fatalf("resolved client IP = %q, want %q", got, test.want)
			}
		})
	}
}
