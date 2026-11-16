package agent

import "testing"

func TestInsecureHTTPRequiresExplicitOptIn(t *testing.T) {
	if _, err := NewClient("http://10.8.1.3:8080"); err == nil {
		t.Fatal("plain HTTP endpoint accepted without explicit opt-in")
	}
	client, err := NewClientAllowInsecureHTTP("http://10.8.1.3:8080")
	if err != nil {
		t.Fatalf("explicit private HTTP opt-in rejected: %v", err)
	}
	if client.baseURL != "http://10.8.1.3:8080" {
		t.Fatalf("client base URL = %q", client.baseURL)
	}
}
