package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestArgonAdmissionHonorsCancellation(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := acquireArgonSlot(ctx, slots); err == nil {
		t.Fatal("cancelled admission succeeded")
	}
}

func TestAuditAllowlistDropsUnapprovedFields(t *testing.T) {
	b, err := redactedJSON(map[string]any{"username": "admin", "api_token": "canary", "opaque": "value"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "canary") || strings.Contains(string(b), "opaque") {
		t.Fatalf("unapproved audit data persisted: %s", b)
	}
}

func TestLoginLimiterBoundsAttemptsAndExpiresWindow(t *testing.T) {
	limiter := newLoginAttemptLimiter(2, 2, time.Minute)
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	limiter.fail("client", now)
	limiter.fail("client", now)
	if limiter.allow("client", now) {
		t.Fatal("third attempt allowed")
	}
	if !limiter.allow("client", now.Add(time.Minute)) {
		t.Fatal("expired window remained blocked")
	}
	limiter.fail("a", now)
	limiter.fail("b", now)
	limiter.fail("c", now)
	if len(limiter.entries) > 2 {
		t.Fatalf("limiter grew to %d entries", len(limiter.entries))
	}
}

func TestLoginSameOriginAndCredentialClass(t *testing.T) {
	r := httptest.NewRequest("POST", "https://example.test/auth/login", nil)
	r.Host = "example.test"
	r.Header.Set("Origin", "https://example.test")
	if !sameOrigin(r) {
		t.Fatal("same origin rejected")
	}
	r.Header.Set("Origin", "https://attacker.test")
	if sameOrigin(r) {
		t.Fatal("cross origin accepted")
	}
	r.Header.Set("Authorization", "Bearer token")
	if !rejectCredentialClass(r, credentialHumanSession) {
		t.Fatal("bearer credential not rejected for human route")
	}
	if (Principal{Class: credentialHumanSession}).HasCapability("audit:read") {
		t.Fatal("human admin implicitly received undeclared scope")
	}
}

func TestAuditRedactsCredentialFieldsRecursively(t *testing.T) {
	b, err := redactedJSON(map[string]any{"password": "pw-sentinel", "nested": map[string]any{"api_token": "token-sentinel", "label": "safe"}})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(b, &value); err != nil {
		t.Fatal(err)
	}
	encoded := string(b)
	for _, sentinel := range []string{"pw-sentinel", "token-sentinel"} {
		if contains(encoded, sentinel) {
			t.Fatalf("audit retained secret sentinel: %s", sentinel)
		}
	}
	if _, exists := value["password"]; exists {
		t.Fatalf("unapproved password field was retained: %s", b)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
