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

type changePasswordAccountFake struct {
	result error
	calls  int
	userID int64
}

func (*changePasswordAccountFake) FindActiveByUsername(
	context.Context,
	string,
) (auth.LoginAccount, error) {
	return auth.LoginAccount{}, sql.ErrNoRows
}

func (f *changePasswordAccountFake) ChangePassword(
	_ context.Context,
	id int64,
	oldPassword string,
	newPassword string,
) error {
	f.calls++
	f.userID = id
	return f.result
}

type changePasswordSessionsFake struct{}

func (*changePasswordSessionsFake) Create(
	context.Context,
	int64,
) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func (*changePasswordSessionsFake) Authenticate(
	context.Context,
	string,
) (auth.SessionIdentity, error) {
	return auth.SessionIdentity{
		UserID:             7,
		Role:               "staff",
		MustChangePassword: true,
	}, nil
}

func (*changePasswordSessionsFake) Revoke(
	context.Context,
	string,
) error {
	return nil
}

func TestChangePasswordHTTP(t *testing.T) {
	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	users := &changePasswordAccountFake{}

	handler, err := NewAuthHandler(
		users,
		&changePasswordSessionsFake{},
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

	token := "session-for-testing-only"

	csrfRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/auth/csrf",
		nil,
	)

	csrfRequest.AddCookie(&http.Cookie{
		Name:  auth.SessionCookieName,
		Value: token,
	})

	csrfResponse := httptest.NewRecorder()
	router.ServeHTTP(csrfResponse, csrfRequest)

	if csrfResponse.Code != http.StatusOK {
		t.Fatalf(
			"csrf status: %d",
			csrfResponse.Code,
		)
	}

	var csrf struct {
		Data struct {
			Token string `json:"csrf_token"`
		} `json:"data"`
	}

	if err := json.Unmarshal(
		csrfResponse.Body.Bytes(),
		&csrf,
	); err != nil {
		t.Fatal(err)
	}

	request := func(
		origin string,
		csrfHeader string,
		body string,
	) *httptest.ResponseRecorder {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/auth/change-password",
			strings.NewReader(body),
		)

		req.Header.Set(
			"Content-Type",
			"application/json",
		)

		if origin != "" {
			req.Header.Set("Origin", origin)
		}

		if csrfHeader != "" {
			req.Header.Set(
				"X-CSRF-Token",
				csrfHeader,
			)
		}

		req.AddCookie(&http.Cookie{
			Name:  auth.SessionCookieName,
			Value: token,
		})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		return rec
	}

	body := `{"current_password":"old-password","new_password":"new-long-password-123456"}`

	if rec := request(
		"",
		csrf.Data.Token,
		body,
	); rec.Code != 403 {
		t.Fatalf(
			"missing Origin: %d",
			rec.Code,
		)
	}

	if rec := request(
		"http://evil.example",
		csrf.Data.Token,
		body,
	); rec.Code != 403 {
		t.Fatalf(
			"bad Origin: %d",
			rec.Code,
		)
	}

	if rec := request(
		"http://127.0.0.1:8080",
		"",
		body,
	); rec.Code != 403 {
		t.Fatalf(
			"missing CSRF: %d",
			rec.Code,
		)
	}

	if rec := request(
		"http://127.0.0.1:8080",
		csrf.Data.Token,
		`{}`,
	); rec.Code != 400 {
		t.Fatalf(
			"invalid body: %d",
			rec.Code,
		)
	}

	if users.calls != 0 {
		t.Fatalf(
			"store called before validation: %d",
			users.calls,
		)
	}

	users.result = auth.ErrCurrentPasswordIncorrect

	if rec := request(
		"http://127.0.0.1:8080",
		csrf.Data.Token,
		body,
	); rec.Code != 403 {
		t.Fatalf(
			"wrong current password: %d",
			rec.Code,
		)
	}

	users.result = auth.ErrPasswordReused

	if rec := request(
		"http://127.0.0.1:8080",
		csrf.Data.Token,
		body,
	); rec.Code != 409 {
		t.Fatalf(
			"reused password: %d",
			rec.Code,
		)
	}

	users.result = nil

	rec := request(
		"http://127.0.0.1:8080",
		csrf.Data.Token,
		body,
	)

	if rec.Code != 204 {
		t.Fatalf(
			"success: %d, body: %s",
			rec.Code,
			rec.Body.String(),
		)
	}

	if users.userID != 7 {
		t.Fatalf(
			"expected user 7, got %d",
			users.userID,
		)
	}

	cookies := rec.Result().Cookies()

	if len(cookies) == 0 || cookies[0].MaxAge != -1 {
		t.Fatalf(
			"missing cleared cookie: %+v",
			cookies,
		)
	}
}
