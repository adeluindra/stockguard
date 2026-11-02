package httpapi

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
	"github.com/adeluindra/stockguard/apps/api/internal/products"
)

type productAccountFake struct{}

func (*productAccountFake) FindActiveByUsername(
	context.Context,
	string,
) (auth.LoginAccount, error) {
	return auth.LoginAccount{}, sql.ErrNoRows
}

type productSessionFake struct {
	role          string
	authenticated bool
	mustChange    bool
}

func (*productSessionFake) Create(
	context.Context,
	int64,
) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func (s *productSessionFake) Authenticate(
	context.Context,
	string,
) (auth.SessionIdentity, error) {
	if !s.authenticated {
		return auth.SessionIdentity{},
			auth.ErrInvalidSession
	}

	return auth.SessionIdentity{
		UserID:             1,
		Role:               s.role,
		MustChangePassword: s.mustChange,
	}, nil
}

func (*productSessionFake) Revoke(
	context.Context,
	string,
) error {
	return nil
}

type productServiceFake struct {
	created     bool
	updated     bool
	deactivated bool
}

func (f *productServiceFake) Get(
	_ context.Context,
	id int64,
) (products.Product, error) {
	if id != 1 {
		return products.Product{}, products.ErrNotFound
	}

	return products.Product{
		ID:       1,
		SKU:      "SKU-001",
		Name:     "Test Product",
		IsActive: !f.deactivated,
	}, nil
}

func (f *productServiceFake) List(
	_ context.Context,
	limit int,
	offset int,
	includeInactive bool,
) (products.Page, error) {
	items := []products.Product{}

	if !f.deactivated || includeInactive {
		p, _ := f.Get(context.Background(), 1)
		items = append(items, p)
	}

	return products.Page{
		Items:  items,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (f *productServiceFake) Create(
	_ context.Context,
	in products.CreateInput,
) (products.Product, error) {
	if err := products.ValidateCreate(in); err != nil {
		return products.Product{}, err
	}

	f.created = true

	return products.Product{
		ID:                1,
		SKU:               in.SKU,
		Name:              in.Name,
		Description:       in.Description,
		LowStockThreshold: in.LowStockThreshold,
		IsActive:          true,
	}, nil
}

func (f *productServiceFake) Update(
	_ context.Context,
	id int64,
	in products.UpdateInput,
) (products.Product, error) {
	if err := products.ValidateUpdate(in); err != nil {
		return products.Product{}, err
	}

	f.updated = true

	return products.Product{
		ID:                id,
		SKU:               "SKU-001",
		Name:              in.Name,
		LowStockThreshold: in.LowStockThreshold,
		IsActive:          true,
	}, nil
}

func (f *productServiceFake) Deactivate(
	context.Context,
	int64,
) error {
	f.deactivated = true
	return nil
}

func TestProductRoutesRBACAndCSRF(t *testing.T) {
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

	store := &productServiceFake{}

	router := NewRouterWithAuthAndProducts(
		func(context.Context) error { return nil },
		logger,
		authHandler,
		store,
	)

	token := strings.Repeat("A", 43)

	request := func(
		method string,
		path string,
		body string,
		withCSRF bool,
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

		if withCSRF {
			req.Header.Set(
				"X-CSRF-Token",
				csrfFromSessionToken(token),
			)
		}

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		return rec
	}

	sessions.authenticated = false

	if rec := request(
		"GET", "/api/v1/products", "", false,
	); rec.Code != 401 {
		t.Fatalf("anonymous list: %d", rec.Code)
	}

	sessions.authenticated = true
	sessions.role = "staff"

	if rec := request(
		"GET", "/api/v1/products", "", false,
	); rec.Code != 200 {
		t.Fatalf("staff list: %d", rec.Code)
	}

	if rec := request(
		"GET",
		"/api/v1/products?include_inactive=true",
		"",
		false,
	); rec.Code != 403 {
		t.Fatalf("staff inactive list: %d", rec.Code)
	}

	createBody := `{
		"sku":"SKU-001",
		"name":"Test Product",
		"low_stock_threshold":5
	}`

	if rec := request(
		"POST",
		"/api/v1/products",
		createBody,
		true,
	); rec.Code != 403 {
		t.Fatalf("staff create: %d", rec.Code)
	}

	sessions.role = "admin"

	if rec := request(
		"POST",
		"/api/v1/products",
		createBody,
		false,
	); rec.Code != 403 {
		t.Fatalf("missing CSRF: %d", rec.Code)
	}

	if rec := request(
		"POST",
		"/api/v1/products",
		`{"sku":"BAD","name":""}`,
		true,
	); rec.Code != 400 {
		t.Fatalf("invalid product: %d", rec.Code)
	}

	created := request(
		"POST",
		"/api/v1/products",
		createBody,
		true,
	)

	if created.Code != 201 || !store.created {
		t.Fatalf(
			"create product: %d %s",
			created.Code,
			created.Body.String(),
		)
	}

	if created.Header().Get("Location") !=
		"/api/v1/products/1" {
		t.Fatal("product Location header missing")
	}

	if rec := request(
		"GET",
		"/api/v1/products/1",
		"",
		false,
	); rec.Code != 200 {
		t.Fatalf("get product: %d", rec.Code)
	}

	updateBody := `{
		"name":"Updated Product",
		"description":null,
		"low_stock_threshold":10
	}`

	if rec := request(
		"PUT",
		"/api/v1/products/1",
		updateBody,
		true,
	); rec.Code != 200 || !store.updated {
		t.Fatalf("update product: %d", rec.Code)
	}

	if rec := request(
		"POST",
		"/api/v1/products/1/deactivate",
		"",
		true,
	); rec.Code != 204 || !store.deactivated {
		t.Fatalf("deactivate product: %d", rec.Code)
	}

	sessions.role = "staff"

	if rec := request(
		"GET",
		"/api/v1/products/1",
		"",
		false,
	); rec.Code != 404 {
		t.Fatalf("inactive product visible to staff: %d", rec.Code)
	}

	sessions.role = "admin"
	sessions.mustChange = true

	if rec := request(
		"GET",
		"/api/v1/products",
		"",
		false,
	); rec.Code != 403 {
		t.Fatalf(
			"mandatory password change bypassed: %d",
			rec.Code,
		)
	}
}
