package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
	"github.com/adeluindra/stockguard/apps/api/internal/dashboard"
)

type fakeDashboardStore struct {
	calls  int
	limit  int
	offset int
	err    error
}

func (f *fakeDashboardStore) GetSummary(
	context.Context,
) (dashboard.Summary, error) {
	f.calls++

	if f.err != nil {
		return dashboard.Summary{}, f.err
	}

	return dashboard.Summary{
		Inventory: dashboard.StockTotals{
			ActiveProducts:   3,
			LowStockProducts: 1,
			OnHand:           20,
			Reserved:         5,
			Available:        15,
		},
		Orders: dashboard.OrderTotals{
			Total:     3,
			Reserved:  1,
			Fulfilled: 1,
			Cancelled: 1,
		},
	}, nil
}

func (f *fakeDashboardStore) ListLowStock(
	_ context.Context,
	limit, offset int,
) (dashboard.Page[dashboard.LowStockProduct], error) {
	f.calls++
	f.limit = limit
	f.offset = offset

	if f.err != nil {
		return dashboard.Page[dashboard.LowStockProduct]{},
			f.err
	}

	return dashboard.Page[dashboard.LowStockProduct]{
		Items: []dashboard.LowStockProduct{
			{
				ProductID:         1,
				SKU:               "TEST-001",
				Name:              "Fixture",
				LowStockThreshold: 5,
				OnHand:            3,
				Reserved:          1,
				Available:         2,
			},
		},
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (f *fakeDashboardStore) ListActivity(
	_ context.Context,
	limit, offset int,
) (dashboard.Page[dashboard.Activity], error) {
	f.calls++
	f.limit = limit
	f.offset = offset

	if f.err != nil {
		return dashboard.Page[dashboard.Activity]{},
			f.err
	}

	return dashboard.Page[dashboard.Activity]{
		Items: []dashboard.Activity{
			{
				ID:            2,
				ProductID:     1,
				ActorUserID:   1,
				ActorUsername: "admin",
				MovementType:  "adjustment",
				DeltaOnHand:   3,
				DeltaReserved: 0,
			},
		},
		Limit:  limit,
		Offset: offset,
	}, nil
}

func TestDashboardRoutes(t *testing.T) {
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

	store := &fakeDashboardStore{}

	router := NewRouterWithDashboard(
		func(context.Context) error { return nil },
		logger,
		authHandler,
		nil,
		nil,
		nil,
		store,
	)

	token := strings.Repeat("A", 43)

	request := func(
		path string,
	) *httptest.ResponseRecorder {
		req := httptest.NewRequest(
			http.MethodGet,
			path,
			nil,
		)

		req.AddCookie(&http.Cookie{
			Name:  auth.SessionCookieName,
			Value: token,
		})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		return rec
	}

	paths := []string{
		"/api/v1/dashboard/summary",
		"/api/v1/dashboard/low-stock",
		"/api/v1/dashboard/activity",
	}

	// Anonymous users must be rejected.
	sessions.authenticated = false

	for _, path := range paths {
		if rec := request(path); rec.Code != 401 {
			t.Fatalf(
				"anonymous access %s: %d",
				path,
				rec.Code,
			)
		}
	}

	// Staff cannot access global dashboard reports.
	sessions.authenticated = true
	sessions.role = "staff"

	for _, path := range paths {
		if rec := request(path); rec.Code != 403 {
			t.Fatalf(
				"staff access %s: %d",
				path,
				rec.Code,
			)
		}
	}

	// Mandatory password change still blocks reports.
	sessions.role = "admin"
	sessions.mustChange = true

	if rec := request(paths[0]); rec.Code != 403 {
		t.Fatalf(
			"password change required: %d",
			rec.Code,
		)
	}

	sessions.mustChange = false

	if store.calls != 0 {
		t.Fatal("service called before authorization")
	}

	// Admin can read the dashboard summary.
	rec := request(paths[0])

	if rec.Code != 200 {
		t.Fatalf(
			"summary status: %d",
			rec.Code,
		)
	}

	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("summary is not marked no-store")
	}

	var summaryResponse struct {
		Data dashboard.Summary `json:"data"`
	}

	if err := json.Unmarshal(
		rec.Body.Bytes(),
		&summaryResponse,
	); err != nil {
		t.Fatal(err)
	}

	if summaryResponse.Data.Inventory.Available != 15 ||
		summaryResponse.Data.Orders.Total != 3 {
		t.Fatalf(
			"incorrect summary: %+v",
			summaryResponse.Data,
		)
	}

	// Pagination values must be forwarded correctly.
	rec = request(
		"/api/v1/dashboard/low-stock?limit=5&offset=2",
	)

	if rec.Code != 200 ||
		store.limit != 5 ||
		store.offset != 2 {
		t.Fatalf(
			"low-stock pagination: %d",
			rec.Code,
		)
	}

	rec = request("/api/v1/dashboard/activity")

	if rec.Code != 200 ||
		store.limit != 20 ||
		store.offset != 0 ||
		!strings.Contains(
			rec.Body.String(),
			`"movement_type":"adjustment"`,
		) {
		t.Fatalf("activity response: %d", rec.Code)
	}

	invalid := []string{
		"/api/v1/dashboard/summary?limit=5",
		"/api/v1/dashboard/low-stock?limit=0",
		"/api/v1/dashboard/low-stock?limit=101",
		"/api/v1/dashboard/low-stock?offset=-1",
		"/api/v1/dashboard/low-stock?limit=",
		"/api/v1/dashboard/low-stock?limit=5&limit=10",
		"/api/v1/dashboard/activity?unknown=1",
		"/api/v1/dashboard/activity?offset=1000001",
	}

	before := store.calls

	for _, path := range invalid {
		rec := request(path)

		if rec.Code != 400 {
			t.Fatalf(
				"invalid query %s: %d",
				path,
				rec.Code,
			)
		}
	}

	if store.calls != before {
		t.Fatal("service called for invalid input")
	}

	// Database errors must not leak internal details.
	store.err = errors.New("simulated database failure")

	rec = request(paths[0])

	if rec.Code != 503 ||
		!strings.Contains(
			rec.Body.String(),
			`"SERVICE_UNAVAILABLE"`,
		) {
		t.Fatalf(
			"database error: %d %s",
			rec.Code,
			rec.Body.String(),
		)
	}
}
