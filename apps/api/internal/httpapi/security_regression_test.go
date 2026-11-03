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
)

func TestBusinessRoutesSecurityRegression(t *testing.T) {
	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	sessions := &productSessionFake{
		authenticated: true,
		role:          "admin",
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

	products := &productServiceFake{}
	inventory := &fakeInventoryStore{}

	orders := &fakeOrderQueryStore{
		fakeTerminalOrders: &fakeTerminalOrders{},
	}

	dashboard := &fakeDashboardStore{}

	router := NewRouterWithDashboard(
		func(context.Context) error { return nil },
		logger,
		authHandler,
		products,
		inventory,
		orders,
		dashboard,
	)

	type testCase struct {
		name          string
		method        string
		path          string
		role          string
		authenticated bool
		mustChange    bool
		withCSRF      bool
		wrongOrigin   bool
		wantStatus    int
	}

	cases := []testCase{
		{
			name:       "anonymous products",
			method:     "GET",
			path:       "/api/v1/products",
			wantStatus: 401,
		},
		{
			name:       "anonymous orders",
			method:     "GET",
			path:       "/api/v1/orders",
			wantStatus: 401,
		},
		{
			name:       "anonymous dashboard",
			method:     "GET",
			path:       "/api/v1/dashboard/summary",
			wantStatus: 401,
		},
		{
			name:          "staff reads products",
			method:        "GET",
			path:          "/api/v1/products",
			role:          "staff",
			authenticated: true,
			wantStatus:    200,
		},
		{
			name:          "staff reads own orders",
			method:        "GET",
			path:          "/api/v1/orders",
			role:          "staff",
			authenticated: true,
			wantStatus:    200,
		},
		{
			name:          "staff reads stock",
			method:        "GET",
			path:          "/api/v1/products/1/inventory",
			role:          "staff",
			authenticated: true,
			wantStatus:    200,
		},
		{
			name:          "staff denied dashboard",
			method:        "GET",
			path:          "/api/v1/dashboard/summary",
			role:          "staff",
			authenticated: true,
			wantStatus:    403,
		},
		{
			name:          "staff denied audit activity",
			method:        "GET",
			path:          "/api/v1/dashboard/activity",
			role:          "staff",
			authenticated: true,
			wantStatus:    403,
		},
		{
			name:          "staff denied movement history",
			method:        "GET",
			path:          "/api/v1/products/1/inventory/movements",
			role:          "staff",
			authenticated: true,
			wantStatus:    403,
		},
		{
			name:          "staff denied product creation",
			method:        "POST",
			path:          "/api/v1/products",
			role:          "staff",
			authenticated: true,
			withCSRF:      true,
			wantStatus:    403,
		},
		{
			name:          "staff denied adjustment",
			method:        "POST",
			path:          "/api/v1/products/1/inventory/adjustments",
			role:          "staff",
			authenticated: true,
			withCSRF:      true,
			wantStatus:    403,
		},
		{
			name:          "staff denied fulfillment",
			method:        "POST",
			path:          "/api/v1/orders/1/fulfill",
			role:          "staff",
			authenticated: true,
			withCSRF:      true,
			wantStatus:    403,
		},
		{
			name:          "staff denied cancellation",
			method:        "POST",
			path:          "/api/v1/orders/1/cancel",
			role:          "staff",
			authenticated: true,
			withCSRF:      true,
			wantStatus:    403,
		},
		{
			name:          "admin must change password",
			method:        "GET",
			path:          "/api/v1/dashboard/summary",
			role:          "admin",
			authenticated: true,
			mustChange:    true,
			wantStatus:    403,
		},
		{
			name:          "admin create without CSRF",
			method:        "POST",
			path:          "/api/v1/orders",
			role:          "admin",
			authenticated: true,
			wantStatus:    403,
		},
		{
			name:          "admin create wrong origin",
			method:        "POST",
			path:          "/api/v1/orders",
			role:          "admin",
			authenticated: true,
			withCSRF:      true,
			wrongOrigin:   true,
			wantStatus:    403,
		},
		{
			name:          "admin reads summary",
			method:        "GET",
			path:          "/api/v1/dashboard/summary",
			role:          "admin",
			authenticated: true,
			wantStatus:    200,
		},
		{
			name:          "admin reads audit activity",
			method:        "GET",
			path:          "/api/v1/dashboard/activity",
			role:          "admin",
			authenticated: true,
			wantStatus:    200,
		},
	}

	token := strings.Repeat("A", 43)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessions.role = tc.role
			sessions.authenticated = tc.authenticated
			sessions.mustChange = tc.mustChange

			req := httptest.NewRequest(
				tc.method,
				tc.path,
				strings.NewReader(
					`{"items":[{"product_id":1,"quantity":1}]}`,
				),
			)

			req.AddCookie(&http.Cookie{
				Name:  auth.SessionCookieName,
				Value: token,
			})

			req.Header.Set(
				"Content-Type",
				"application/json",
			)

			req.Header.Set(
				"Idempotency-Key",
				"11111111-1111-4111-8111-111111111111",
			)

			origin := "http://127.0.0.1:8080"
			if tc.wrongOrigin {
				origin = "http://untrusted.example"
			}

			req.Header.Set("Origin", origin)

			if tc.withCSRF {
				req.Header.Set(
					"X-CSRF-Token",
					csrfFromSessionToken(token),
				)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf(
					"status=%d want=%d response=%s",
					rec.Code,
					tc.wantStatus,
					rec.Body.String(),
				)
			}
		})
	}

	// Denied write requests must not reach the
	// product or inventory mutation services.
	if products.created ||
		products.updated ||
		products.deactivated ||
		inventory.adjusted {
		t.Fatal("unauthorized mutation reached service")
	}
}
