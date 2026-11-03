package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
	"github.com/adeluindra/stockguard/apps/api/internal/inventory"
)

// Add the new method to the existing BE-4 mock.
func (s *fakeInventoryStore) AdjustIdempotent(
	ctx context.Context,
	productID, actorID, delta int64,
	reason, key string,
) (inventory.IdempotentAdjustment, error) {
	result, err := s.Adjust(
		ctx, productID, actorID, delta, reason,
	)
	if err != nil {
		return inventory.IdempotentAdjustment{}, err
	}

	return inventory.IdempotentAdjustment{
		AdjustmentResult: result,
	}, nil
}

type fakeIdempotentStock struct {
	*fakeInventoryStore
	calls      int
	lastKey    string
	lastDelta  int64
	lastReason string
	snapshot   inventory.AdjustmentResult
}

func (f *fakeIdempotentStock) AdjustIdempotent(
	ctx context.Context,
	productID, actorID, delta int64,
	reason, key string,
) (inventory.IdempotentAdjustment, error) {
	if key == f.lastKey {
		if delta != f.lastDelta ||
			reason != f.lastReason {
			return inventory.IdempotentAdjustment{},
				inventory.ErrAdjustmentKeyConflict
		}

		return inventory.IdempotentAdjustment{
			AdjustmentResult: f.snapshot,
			Replayed:         true,
		}, nil
	}

	result, err := f.fakeInventoryStore.Adjust(
		ctx, productID, actorID, delta, reason,
	)
	if err != nil {
		return inventory.IdempotentAdjustment{}, err
	}

	f.calls++
	f.lastKey = key
	f.lastDelta = delta
	f.lastReason = reason
	f.snapshot = result

	return inventory.IdempotentAdjustment{
		AdjustmentResult: result,
		Replayed:         false,
	}, nil
}

func TestInventoryIdempotencyHTTP(t *testing.T) {
	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	sessions := &productSessionFake{
		role:          "admin",
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

	store := &fakeIdempotentStock{
		fakeInventoryStore: &fakeInventoryStore{},
	}

	mux := http.NewServeMux()
	RegisterInventoryRoutes(mux, authHandler, store)

	handler := requestLogging(logger, mux)

	const path = "/api/v1/products/1/inventory/adjustments"
	const key = "71fa94a1-8c33-45a6-b915-ecf719cf48c1"
	const body = `{"delta":2,"reason":"Receive supplier shipment"}`

	token := strings.Repeat("A", 43)

	send := func(
		keyValue, payload string,
		withCSRF bool,
	) *httptest.ResponseRecorder {
		req := httptest.NewRequest(
			http.MethodPost,
			path,
			strings.NewReader(payload),
		)

		req.AddCookie(&http.Cookie{
			Name:  auth.SessionCookieName,
			Value: token,
		})

		req.Header.Set(
			"Origin", "http://127.0.0.1:8080",
		)
		req.Header.Set(
			"Content-Type", "application/json",
		)

		if keyValue != "" {
			req.Header.Set("Idempotency-Key", keyValue)
		}

		if withCSRF {
			req.Header.Set(
				"X-CSRF-Token",
				csrfFromSessionToken(token),
			)
		}

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	sessions.authenticated = false
	if rec := send(key, body, true); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}

	sessions.authenticated = true
	sessions.role = "staff"
	if rec := send(key, body, true); rec.Code != 403 {
		t.Fatalf("staff: %d", rec.Code)
	}

	sessions.role = "admin"

	if rec := send(key, body, false); rec.Code != 403 {
		t.Fatalf("missing CSRF: %d", rec.Code)
	}

	if rec := send("", body, true); rec.Code != 400 {
		t.Fatalf("missing key: %d", rec.Code)
	}

	if rec := send("invalid", body, true); rec.Code != 400 {
		t.Fatalf("invalid key: %d", rec.Code)
	}

	if store.calls != 0 {
		t.Fatal("service called for invalid request")
	}

	rec := send(key, body, true)
	if rec.Code != 200 ||
		!strings.Contains(
			rec.Body.String(),
			`"replayed":false`,
		) {
		t.Fatalf("first adjustment: %d %s",
			rec.Code, rec.Body.String())
	}

	rec = send(strings.ToUpper(key), body, true)
	if rec.Code != 200 ||
		!strings.Contains(
			rec.Body.String(),
			`"replayed":true`,
		) ||
		store.calls != 1 {
		t.Fatalf("replay: %d %s",
			rec.Code, rec.Body.String())
	}

	conflicting := `{"delta":3,"reason":"Receive supplier shipment"}`

	rec = send(key, conflicting, true)
	if rec.Code != 409 || store.calls != 1 {
		t.Fatalf("key conflict: %d", rec.Code)
	}
}
