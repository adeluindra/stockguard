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

type fakeInventoryStore struct {
	adjusted bool
	delta    int64
	actorID  int64
	inactive bool
}

func (s *fakeInventoryStore) GetBalance(
	_ context.Context,
	productID int64,
) (inventory.Balance, error) {
	if productID != 1 {
		return inventory.Balance{},
			inventory.ErrProductNotFound
	}

	return inventory.Balance{
		ProductID: 1,
		SKU:       "TEST-001",
		Name:      "Fixture",
		IsActive:  !s.inactive,
		OnHand:    10,
		Reserved:  4,
		Available: 6,
	}, nil
}

func (s *fakeInventoryStore) Adjust(
	_ context.Context,
	productID int64,
	actorID int64,
	delta int64,
	reason string,
) (inventory.AdjustmentResult, error) {
	if productID != 1 {
		return inventory.AdjustmentResult{},
			inventory.ErrProductNotFound
	}

	if delta < -6 {
		return inventory.AdjustmentResult{},
			inventory.ErrInsufficientAvailable
	}

	s.adjusted = true
	s.delta = delta
	s.actorID = actorID

	b, _ := s.GetBalance(context.Background(), 1)
	b.OnHand += delta
	b.Available += delta

	return inventory.AdjustmentResult{
		Balance: b,
		Movement: inventory.Movement{
			ID:           3,
			ProductID:    productID,
			ActorUserID:  actorID,
			MovementType: "adjustment",
			DeltaOnHand:  delta,
			Reason:       &reason,
		},
	}, nil
}

func (s *fakeInventoryStore) ListMovements(
	_ context.Context,
	productID int64,
	limit int,
	offset int,
) (inventory.MovementPage, error) {
	if productID != 1 {
		return inventory.MovementPage{},
			inventory.ErrProductNotFound
	}

	return inventory.MovementPage{
		Items: []inventory.Movement{
			{
				ID:           3,
				ProductID:    1,
				ActorUserID:  1,
				MovementType: "adjustment",
				DeltaOnHand:  1,
			},
		},
		Limit:  limit,
		Offset: offset,
	}, nil
}

func TestInventoryRoutesRBACAndCSRF(t *testing.T) {
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

	store := &fakeInventoryStore{}

	mux := http.NewServeMux()
	RegisterInventoryRoutes(mux, authHandler, store)

	handler := requestLogging(logger, mux)
	token := strings.Repeat("A", 43)

	request := func(
		method, path, body string,
		csrf bool,
	) *httptest.ResponseRecorder {
		req := httptest.NewRequest(
			method,
			path,
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

		if csrf {
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

	if rec := request(
		"GET",
		"/api/v1/products/1/inventory",
		"",
		false,
	); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}

	sessions.authenticated = true

	if rec := request(
		"GET",
		"/api/v1/products/1/inventory",
		"",
		false,
	); rec.Code != 200 {
		t.Fatalf("staff read: %d", rec.Code)
	}

	if rec := request(
		"POST",
		"/api/v1/products/1/inventory/adjustments",
		`{"delta":1,"reason":"Stock count adjustment"}`,
		true,
	); rec.Code != 403 {
		t.Fatalf("staff write: %d", rec.Code)
	}

	if rec := request(
		"GET",
		"/api/v1/products/1/inventory/movements",
		"",
		false,
	); rec.Code != 403 {
		t.Fatalf("staff history: %d", rec.Code)
	}

	sessions.role = "admin"

	if rec := request(
		"POST",
		"/api/v1/products/1/inventory/adjustments",
		`{"delta":1,"reason":"Stock count adjustment"}`,
		false,
	); rec.Code != 403 {
		t.Fatalf("missing CSRF: %d", rec.Code)
	}

	if rec := request(
		"POST",
		"/api/v1/products/1/inventory/adjustments",
		`{"delta":0,"reason":"Stock count adjustment"}`,
		true,
	); rec.Code != 400 {
		t.Fatalf("zero delta: %d", rec.Code)
	}

	if rec := request(
		"POST",
		"/api/v1/products/1/inventory/adjustments",
		`{"delta":-7,"reason":"Stock count adjustment"}`,
		true,
	); rec.Code != 409 {
		t.Fatalf("reserved invariant: %d", rec.Code)
	}

	if rec := request(
		"POST",
		"/api/v1/products/1/inventory/adjustments",
		`{"delta":-6,"reason":"Stock count adjustment"}`,
		true,
	); rec.Code != 200 ||
		!store.adjusted ||
		store.delta != -6 ||
		store.actorID != 1 {
		t.Fatalf("valid adjustment: %d", rec.Code)
	}

	if rec := request(
		"GET",
		"/api/v1/products/1/inventory/movements?limit=5&offset=0",
		"",
		false,
	); rec.Code != 200 {
		t.Fatalf("admin history: %d", rec.Code)
	}

	if rec := request(
		"GET",
		"/api/v1/products/1/inventory/movements?limit=101",
		"",
		false,
	); rec.Code != 400 {
		t.Fatalf("invalid pagination: %d", rec.Code)
	}

	store.inactive = true
	sessions.role = "staff"

	if rec := request(
		"GET",
		"/api/v1/products/1/inventory",
		"",
		false,
	); rec.Code != 404 {
		t.Fatalf(
			"inactive leaked to staff: %d",
			rec.Code,
		)
	}
}
