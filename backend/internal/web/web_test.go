package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedFrontendHandler(t *testing.T) {
	handler := Handler()

	// 1. Root route "/" should serve index.html
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for root, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<!doctype html>") && !strings.Contains(body, "<html") {
		t.Fatalf("expected HTML in response, got: %s", body)
	}

	// 2. Client-side route "/dashboard" should fallback to index.html
	req = httptest.NewRequest("GET", "/dashboard", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /dashboard fallback, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<!doctype html>") && !strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("expected HTML fallback for /dashboard, got: %s", w.Body.String())
	}
}
