package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"portinaia/internal/database"
)

func TestAuditEventsRouteRequiresSessionAndContinuesFilteredPages(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/audit-route.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapAdministrator(db, "admin", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		id, err := randomID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO audit_events(id,occurred_at,actor_type,actor_id,action,resource_type,result) VALUES(?,?,?,?,?,?,?)`,
			id, "2026-01-01T00:00:0"+strconv.Itoa(i)+"Z", "system", systemActorID, "test.action", "test", "success"); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(db)
	unauthenticated := httptest.NewRecorder()
	h.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/audit-events", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated audit status = %d", unauthenticated.Code)
	}
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"a-long-test-password"}`))
	login.Header.Set("Origin", "http://example.com")
	loginResponse := httptest.NewRecorder()
	h.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d", loginResponse.Code)
	}
	cookie := loginResponse.Result().Cookies()[0]
	for _, query := range []string{"actor_id=not-a-uuid", "from_time=not-a-time", "actor_type=unknown"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/audit-events?"+query, nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("invalid audit query %q status = %d", query, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit-events?page[size]=2&action=test.action", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("audit status = %d: %s", response.Code, response.Body.String())
	}
	var page AuditPage
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.Page.HasMore || page.Page.NextCursor == nil {
		t.Fatalf("first audit page = %+v", page)
	}
	next := url.QueryEscape(*page.Page.NextCursor)
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit-events?page[size]=2&action=test.action&page[after]="+next, nil)
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("continued audit status = %d", response.Code)
	}
	page = AuditPage{}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Page.HasMore {
		t.Fatalf("continued audit page = %+v", page)
	}
}
