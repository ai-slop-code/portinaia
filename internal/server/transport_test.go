package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsUnknownAndTrailingValues(t *testing.T) {
	type payload struct {
		Name     string   `json:"name"`
		Optional *string  `json:"optional" nullable:"true"`
		Values   []string `json:"values"`
	}
	for _, body := range []string{`{"name":"ok","extra":true}`, `{"name":"ok"} {"name":"later"}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var value payload
		if err := decodeJSON(recorder, request, 1024, &value); err == nil {
			t.Errorf("decodeJSON accepted invalid body %s", body)
		}
	}
	for _, body := range []string{`{"name":null}`, `{"name":"ok","values":[null]}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var value payload
		if err := decodeJSON(recorder, request, 1024, &value); err == nil {
			t.Errorf("decodeJSON accepted nonnullable null in body %s", body)
		}
	}
	type nonnullablePointer struct {
		Value *string `json:"value"`
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"value":null}`))
	var invalid nonnullablePointer
	if err := decodeJSON(recorder, request, 1024, &invalid); err == nil {
		t.Fatal("decodeJSON accepted null for an unmarked optional pointer")
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"ok","optional":null,"values":[]}`))
	var value payload
	if err := decodeJSON(recorder, request, 1024, &value); err != nil {
		t.Fatalf("decodeJSON rejected nullable null: %v", err)
	}
}

func TestGeneratedDomainWriteTransportDecodesCalendarDatesAndNullables(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"example.test","registrar_provider_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","registered_on":"2024-01-01","expires_on":"2027-01-01","auto_renew":false,"nameservers":[],"description":"owned","notes":"","tag_ids":[]}`))
	input, err := decodeDomainWrite(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatalf("decode generated DomainWrite transport: %v", err)
	}
	if input.Name != "example.test" || input.RegisteredOn == nil || *input.RegisteredOn != "2024-01-01" || input.ExpiresOn == nil || *input.ExpiresOn != "2027-01-01" {
		t.Fatalf("decoded domain transport = %+v", input)
	}
}
