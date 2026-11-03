package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
	"github.com/adeluindra/stockguard/apps/api/internal/orders"
)

type fakeOrderQueryStore struct {
	*fakeTerminalOrders

	listCalls int
	getCalls  int
	userID    int64
	isAdmin   bool
	status    string
	limit     int
	offset    int
	err       error
}

func (f *fakeOrderQueryStore) List(
	_ context.Context,
	userID int64,
	isAdmin bool,
	status string,
	limit, offset int,
) (orders.OrderPage, error) {
	f.listCalls++
	f.userID = userID
	f.isAdmin = isAdmin
	f.status = status
	f.limit = limit
	f.offset = offset

	if f.err != nil {
		return orders.OrderPage{}, f.err
	}

	return orders.OrderPage{
		Items: []orders.OrderSummary{
			{
				ID:        42,
				CreatedBy: userID,
				Status:    "reserved",
				ItemCount: 2,
			},
		},
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (f *fakeOrderQueryStore) Get(
	_ context.Context,
	userID int64,
	isAdmin bool,
	id int64,
) (orders.Order, error) {
	f.getCalls++
	f.userID = userID
	f.isAdmin = isAdmin

	if f.err != nil {
		return orders.Order{}, f.err
	}

	if id == 99 && !isAdmin {
		return orders.Order{}, orders.ErrOrderNotFound
	}

	return orders.Order{
		ID:        id,
		CreatedBy: userID,
		Status:    "reserved",
		Items: []orders.Item{
			{ProductID: 3, Quantity: 2},
			{ProductID: 7, Quantity: 3},
		},
	}, nil
}

func TestOrderQueryHTTP(t *testing.T) {
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

	store := &fakeOrderQueryStore{
		fakeTerminalOrders: &fakeTerminalOrders{},
	}

	router := NewRouterWithAuthProductsInventoryOrders(
		func(context.Context) error { return nil },
		logger,
		authHandler,
		nil,
		nil,
		store,
	)

	token := strings.Repeat("A", 43)

	send := func(path string) *httptest.ResponseRecorder {
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

	sessions.authenticated = false

	if rec := send(
		"/api/v1/orders",
	); rec.Code != 401 {
		t.Fatalf("anonymous list: %d", rec.Code)
	}

	sessions.authenticated = true

	if rec := send(
		"/api/v1/orders?status=reserved&limit=5&offset=2",
	); rec.Code != 200 {
		t.Fatalf("staff list: %d", rec.Code)
	}

	if store.userID != 1 ||
		store.isAdmin ||
		store.status != "reserved" ||
		store.limit != 5 ||
		store.offset != 2 {
		t.Fatalf("incorrect query scope: %+v", store)
	}

	if rec := send(
		"/api/v1/orders/42",
	); rec.Code != 200 ||
		!strings.Contains(
			rec.Body.String(),
			`"quantity":3`,
		) {
		t.Fatalf("staff detail: %d", rec.Code)
	}

	// A Staff user must not see another user's order.
	if rec := send(
		"/api/v1/orders/99",
	); rec.Code != 404 {
		t.Fatalf("foreign order: %d", rec.Code)
	}

	invalidPaths := []string{
		"/api/v1/orders?status=invalid",
		"/api/v1/orders?status=",
		"/api/v1/orders?limit=0",
		"/api/v1/orders?limit=101",
		"/api/v1/orders?offset=-1",
		"/api/v1/orders?unexpected=1",
		"/api/v1/orders?limit=5&limit=10",
		"/api/v1/orders/0",
		"/api/v1/orders/abc",
		"/api/v1/orders/42?unexpected=1",
	}

	callsBefore := store.listCalls + store.getCalls

	for _, path := range invalidPaths {
		if rec := send(path); rec.Code != 400 {
			t.Fatalf(
				"invalid path %s: status=%d",
				path, rec.Code,
			)
		}
	}

	if store.listCalls+store.getCalls != callsBefore {
		t.Fatal("query service called for invalid input")
	}

	// Password change remains mandatory.
	sessions.mustChange = true

	if rec := send(
		"/api/v1/orders",
	); rec.Code != 403 {
		t.Fatalf("mandatory password change: %d", rec.Code)
	}

	sessions.mustChange = false
	sessions.role = "admin"

	if rec := send(
		"/api/v1/orders?status=fulfilled",
	); rec.Code != 200 || !store.isAdmin {
		t.Fatalf("admin list: %d", rec.Code)
	}

	if rec := send(
		"/api/v1/orders/99",
	); rec.Code != 200 || !store.isAdmin {
		t.Fatalf("admin detail: %d", rec.Code)
	}

	store.err = orders.ErrOrderNotFound

	if rec := send(
		"/api/v1/orders/999",
	); rec.Code != 404 {
		t.Fatalf("missing order: %d", rec.Code)
	}

	store.err = errors.New("simulated database failure")

	if rec := send(
		"/api/v1/orders",
	); rec.Code != 503 {
		t.Fatalf("database failure: %d", rec.Code)
	}
}
