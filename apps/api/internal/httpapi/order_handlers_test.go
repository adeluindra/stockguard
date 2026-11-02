package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
	"github.com/adeluindra/stockguard/apps/api/internal/orders"
)

type fakeOrderService struct {
	calls    int
	actorID  int64
	key      string
	err      error
	replayed bool
}

func (f *fakeOrderService) Create(
	_ context.Context,
	actorID int64,
	key string,
	items []orders.Item,
) (orders.CreateResult, error) {
	f.calls++
	f.actorID = actorID
	f.key = key

	if f.err != nil {
		return orders.CreateResult{}, f.err
	}

	return orders.CreateResult{
		Order: orders.Order{
			ID:        42,
			CreatedBy: actorID,
			Status:    "reserved",
			Items:     items,
			CreatedAt: time.Now(),
		},
		Replayed: f.replayed,
	}, nil
}

func TestOrderCreateHTTP(t *testing.T) {
	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	sessions := &productSessionFake{
		role:          "staff",
		authenticated: true,
	}

	authHandler, err := NewAuthHandler(
		&productAccountFake{},
		sessions,
		logger,
		"http://127.0.0.1:8080",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	store := &fakeOrderService{}

	router := NewRouterWithAuthProductsInventoryOrders(
		func(context.Context) error { return nil },
		logger,
		authHandler,
		nil,
		nil,
		store,
	)

	const key = "3b6d0429-82c1-4b4f-b20f-2da29f1b8170"
	token := strings.Repeat("A", 43)

	request := func(
		body string,
		withCSRF bool,
		withKey bool,
	) *httptest.ResponseRecorder {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/orders",
			strings.NewReader(body),
		)

		req.AddCookie(&http.Cookie{
			Name:  auth.SessionCookieName,
			Value: token,
		})

		req.Header.Set(
			"Origin",
			"http://127.0.0.1:8080",
		)

		req.Header.Set(
			"Content-Type",
			"application/json",
		)

		if withCSRF {
			req.Header.Set(
				"X-CSRF-Token",
				csrfFromSessionToken(token),
			)
		}

		if withKey {
			req.Header.Set("Idempotency-Key", key)
		}

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	body := `{"items":[{"product_id":1,"quantity":3}]}`

	// Authentication is mandatory.
	sessions.authenticated = false

	if rec := request(body, true, true); rec.Code != 401 {
		t.Fatalf("anonymous create: %d", rec.Code)
	}

	sessions.authenticated = true

	// CSRF is mandatory.
	if rec := request(body, false, true); rec.Code != 403 {
		t.Fatalf("missing CSRF: %d", rec.Code)
	}

	// Idempotency key is mandatory.
	if rec := request(body, true, false); rec.Code != 400 {
		t.Fatalf("missing key: %d", rec.Code)
	}

	// Invalid JSON is rejected before service invocation.
	if rec := request(`{"invalid":true}`, true, true); rec.Code != 400 {
		t.Fatalf("invalid JSON: %d", rec.Code)
	}

	if store.calls != 0 {
		t.Fatal("store called before request validation")
	}

	// A Staff user may create orders.
	rec := request(body, true, true)

	if rec.Code != 201 {
		t.Fatalf(
			"staff create: %d %s",
			rec.Code,
			rec.Body.String(),
		)
	}

	if store.actorID != 1 || store.key != key {
		t.Fatal("actor identity or idempotency key incorrect")
	}

	if rec.Header().Get("Location") != "/api/v1/orders/42" {
		t.Fatal("order Location header incorrect")
	}

	// Replaying the same request returns HTTP 200.
	store.replayed = true

	rec = request(body, true, true)

	if rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"replayed":true`) {
		t.Fatalf("idempotent replay: %d", rec.Code)
	}

	// A reused key with different payload is a conflict.
	store.err = orders.ErrIdempotencyConflict

	if rec := request(body, true, true); rec.Code != 409 {
		t.Fatalf("idempotency conflict: %d", rec.Code)
	}

	store.err = orders.ErrInsufficientStock

	if rec := request(body, true, true); rec.Code != 409 {
		t.Fatalf("insufficient stock: %d", rec.Code)
	}

	store.err = nil

	// Mandatory password change blocks business endpoints.
	sessions.mustChange = true

	if rec := request(body, true, true); rec.Code != 403 {
		t.Fatalf(
			"mandatory password change bypassed: %d",
			rec.Code,
		)
	}

	// Admin is also allowed.
	sessions.mustChange = false
	sessions.role = "admin"

	if rec := request(body, true, true); rec.Code != 200 {
		t.Fatalf("admin order create: %d", rec.Code)
	}
}
