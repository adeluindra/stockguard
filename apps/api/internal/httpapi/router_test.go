package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestRouter(check ReadinessCheck) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(check, logger)
}

func TestHealthz(t *testing.T) {
	// Liveness must not depend on PostgreSQL availability.
	router := newTestRouter(func(context.Context) error { return errors.New("db offline") })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var body struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Data.Status != "ok" {
		t.Fatalf("unexpected response: %s (parse error: %v)", rec.Body.String(), err)
	}
}

func TestReadyzHealthy(t *testing.T) {
	router := newTestRouter(func(context.Context) error { return nil })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID header")
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected Content-Type: %q", rec.Header().Get("Content-Type"))
	}
}

func TestReadyzUnavailable(t *testing.T) {
	router := newTestRouter(func(context.Context) error { return errors.New("db offline") })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want 503", rec.Code)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Error.Code != "SERVICE_UNAVAILABLE" {
		t.Fatalf("unexpected error code: %q", body.Error.Code)
	}
	if body.Error.RequestID == "" || body.Error.RequestID != rec.Header().Get("X-Request-ID") {
		t.Fatal("request ID is missing or inconsistent")
	}
}

func TestUnknownPathIs404(t *testing.T) {
	router := newTestRouter(func(context.Context) error { return nil })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rec.Code)
	}
}
