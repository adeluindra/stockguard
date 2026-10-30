package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
)

type fakeStaffAdmin struct {
	created     bool
	reset       bool
	deactivated bool
}

func (*fakeStaffAdmin) FindActiveByUsername(
	context.Context,
	string,
) (auth.LoginAccount, error) {
	return auth.LoginAccount{}, sql.ErrNoRows
}

func (*fakeStaffAdmin) ListStaff(
	context.Context,
) ([]auth.StaffUser, error) {
	return []auth.StaffUser{
		{
			ID:       12,
			Username: "staff_12",
			Role:     "staff",
			IsActive: true,
		},
	}, nil
}

func (f *fakeStaffAdmin) CreateStaff(
	_ context.Context,
	username string,
) (auth.StaffUser, string, error) {
	f.created = true

	return auth.StaffUser{
		ID:                 13,
		Username:           username,
		Role:               "staff",
		IsActive:           true,
		MustChangePassword: true,
	}, "test-only-temp-password", nil
}

func (f *fakeStaffAdmin) ResetStaffPassword(
	context.Context,
	int64,
) (string, error) {
	f.reset = true
	return "test-only-new-temp", nil
}

func (f *fakeStaffAdmin) DeactivateStaff(
	context.Context,
	int64,
) error {
	f.deactivated = true
	return nil
}

type fakeStaffSession struct {
	role       string
	mustChange bool
	valid      bool
}

func (*fakeStaffSession) Create(
	context.Context,
	int64,
) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func (f *fakeStaffSession) Authenticate(
	context.Context,
	string,
) (auth.SessionIdentity, error) {
	if !f.valid {
		return auth.SessionIdentity{}, auth.ErrInvalidSession
	}

	return auth.SessionIdentity{
		UserID:             1,
		Role:               f.role,
		MustChangePassword: f.mustChange,
	}, nil
}

func (*fakeStaffSession) Revoke(
	context.Context,
	string,
) error {
	return nil
}

func TestAdminStaffRoutes(t *testing.T) {
	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	accounts := &fakeStaffAdmin{}
	sessions := &fakeStaffSession{
		role:  "admin",
		valid: true,
	}

	handler, err := NewAuthHandler(
		accounts,
		sessions,
		logger,
		"http://127.0.0.1:8080",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouterWithAuth(
		func(context.Context) error { return nil },
		logger,
		handler,
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

	// No session: access denied.
	sessions.valid = false

	if rec := request(
		"GET", "/api/v1/admin/staff", "", false,
	); rec.Code != 401 {
		t.Fatalf("unauthenticated: %d", rec.Code)
	}

	// Staff cannot access Admin endpoints.
	sessions.valid = true
	sessions.role = "staff"

	if rec := request(
		"GET", "/api/v1/admin/staff", "", false,
	); rec.Code != 403 {
		t.Fatalf("staff role: %d", rec.Code)
	}

	// Admin must finish mandatory password change.
	sessions.role = "admin"
	sessions.mustChange = true

	if rec := request(
		"GET", "/api/v1/admin/staff", "", false,
	); rec.Code != 403 {
		t.Fatalf("password change required: %d", rec.Code)
	}

	sessions.mustChange = false

	if rec := request(
		"GET", "/api/v1/admin/staff", "", false,
	); rec.Code != 200 {
		t.Fatalf("admin list: %d", rec.Code)
	}

	// Missing CSRF must block creation.
	if rec := request(
		"POST",
		"/api/v1/admin/staff",
		`{"username":"newstaff"}`,
		false,
	); rec.Code != 403 {
		t.Fatalf("missing CSRF: %d", rec.Code)
	}

	// Invalid input must be rejected.
	if rec := request(
		"POST",
		"/api/v1/admin/staff",
		`{"username":"no space allowed"}`,
		true,
	); rec.Code != 400 {
		t.Fatalf("invalid username: %d", rec.Code)
	}

	created := request(
		"POST",
		"/api/v1/admin/staff",
		`{"username":"newstaff"}`,
		true,
	)

	if created.Code != 201 {
		t.Fatalf("create: %d", created.Code)
	}

	var data struct {
		Data struct {
			TemporaryPassword string         `json:"temporary_password"`
			User              auth.StaffUser `json:"user"`
		} `json:"data"`
	}

	if err := json.Unmarshal(
		created.Body.Bytes(), &data,
	); err != nil {
		t.Fatal(err)
	}

	if data.Data.TemporaryPassword == "" ||
		data.Data.User.Role != "staff" ||
		!accounts.created {
		t.Fatal("create response missing staff details")
	}

	if rec := request(
		"POST",
		"/api/v1/admin/staff/nope/reset-password",
		"",
		true,
	); rec.Code != 400 {
		t.Fatalf("bad staff ID: %d", rec.Code)
	}

	if rec := request(
		"POST",
		"/api/v1/admin/staff/13/reset-password",
		"",
		true,
	); rec.Code != 200 || !accounts.reset {
		t.Fatalf("reset: %d", rec.Code)
	}

	if rec := request(
		"POST",
		"/api/v1/admin/staff/13/deactivate",
		"",
		true,
	); rec.Code != 204 || !accounts.deactivated {
		t.Fatalf("deactivate: %d", rec.Code)
	}
}
