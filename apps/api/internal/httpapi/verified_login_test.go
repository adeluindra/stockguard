package httpapi

import (
	"context"
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

// Extend the existing fakeSessions defined in
// auth_handlers_test.go without modifying that file.
func (s *fakeSessions) CreateVerified(
	ctx context.Context,
	userID int64,
	verifiedHash string,
) (string, time.Time, error) {
	if verifiedHash == "" {
		return "", time.Time{}, auth.ErrCredentialsChanged
	}

	return s.Create(ctx, userID)
}

// Simulate a password reset completing immediately
// before the session creation step.
type staleLoginSessions struct {
	*fakeSessions
}

func (s *staleLoginSessions) CreateVerified(
	context.Context,
	int64,
	string,
) (string, time.Time, error) {
	return "", time.Time{}, auth.ErrCredentialsChanged
}

func TestLoginRejectsChangedCredentials(t *testing.T) {
	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	passwordHash, err := auth.HashPassword(
		"correct-test-password",
	)
	if err != nil {
		t.Fatal(err)
	}

	sessions := &staleLoginSessions{
		fakeSessions: &fakeSessions{
			role: "staff",
		},
	}

	handler, err := NewAuthHandler(
		fakeAccountFinder{
			user: auth.LoginAccount{
				ID:           10,
				Username:     "teststaff",
				PasswordHash: passwordHash,
				Role:         "staff",
			},
		},
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

	// Obtain CSRF token and cookie.
	csrfRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/auth/login-csrf",
		nil,
	)

	csrfRecorder := httptest.NewRecorder()
	router.ServeHTTP(csrfRecorder, csrfRequest)

	if csrfRecorder.Code != http.StatusOK {
		t.Fatalf(
			"login csrf status: %d",
			csrfRecorder.Code,
		)
	}

	var csrfResponse struct {
		Data struct {
			Token string `json:"csrf_token"`
		} `json:"data"`
	}

	if err := json.Unmarshal(
		csrfRecorder.Body.Bytes(),
		&csrfResponse,
	); err != nil {
		t.Fatal(err)
	}

	var csrfCookie *http.Cookie

	for _, cookie := range csrfRecorder.Result().Cookies() {
		if cookie.Name == loginCSRFCookie {
			csrfCookie = cookie
			break
		}
	}

	if csrfCookie == nil || csrfResponse.Data.Token == "" {
		t.Fatal("missing login csrf cookie or token")
	}

	loginRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login",
		strings.NewReader(
			`{"username":"teststaff","password":"correct-test-password"}`,
		),
	)

	loginRequest.Header.Set(
		"Content-Type",
		"application/json",
	)
	loginRequest.Header.Set(
		"Origin",
		"http://127.0.0.1:8080",
	)
	loginRequest.Header.Set(
		"X-CSRF-Token",
		csrfResponse.Data.Token,
	)

	loginRequest.AddCookie(csrfCookie)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, loginRequest)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"stale password login: got %d, want 401",
			recorder.Code,
		)
	}

	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&response,
	); err != nil {
		t.Fatal(err)
	}

	if response.Error.Code != "INVALID_CREDENTIALS" {
		t.Fatalf(
			"unexpected public error code: %s",
			response.Error.Code,
		)
	}

	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName &&
			cookie.Value != "" {
			t.Fatal("session issued after credentials changed")
		}
	}
}
