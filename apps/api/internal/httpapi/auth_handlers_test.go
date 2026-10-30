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

type fakeAccountFinder struct {
	user auth.LoginAccount
}

func (f fakeAccountFinder) FindActiveByUsername(
	_ context.Context,
	username string,
) (auth.LoginAccount, error) {
	if username != f.user.Username {
		return auth.LoginAccount{}, sql.ErrNoRows
	}
	return f.user, nil
}

type fakeSessions struct {
	revoked    bool
	role       string
	mustChange bool
}

func (s *fakeSessions) Create(
	_ context.Context,
	_ int64,
) (string, time.Time, error) {
	return strings.Repeat("a", 43),
		time.Now().Add(time.Hour), nil
}

func (s *fakeSessions) Authenticate(
	_ context.Context,
	_ string,
) (auth.SessionIdentity, error) {
	if s.revoked {
		return auth.SessionIdentity{},
			auth.ErrInvalidSession
	}

	return auth.SessionIdentity{
		UserID:             1,
		Role:               s.role,
		MustChangePassword: s.mustChange,
		ExpiresAt:          time.Now().Add(time.Hour),
	}, nil
}

func (s *fakeSessions) Revoke(
	_ context.Context,
	_ string,
) error {
	s.revoked = true
	return nil
}

func testAuthRouter(
	t *testing.T,
) (http.Handler, *fakeSessions) {
	t.Helper()

	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	sessions := &fakeSessions{role: "staff"}

	handler, err := NewAuthHandler(
		fakeAccountFinder{
			user: auth.LoginAccount{
				ID:           1,
				Username:     "staff",
				PasswordHash: hash,
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

	return router, sessions
}

func TestLoginCSRFAndLogout(t *testing.T) {
	router, sessions := testAuthRouter(t)

	csrfReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/auth/login-csrf",
		nil,
	)

	csrfRec := httptest.NewRecorder()
	router.ServeHTTP(csrfRec, csrfReq)

	if csrfRec.Code != http.StatusOK {
		t.Fatalf("csrf status: %d", csrfRec.Code)
	}

	var csrfResponse struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}

	if err := json.Unmarshal(
		csrfRec.Body.Bytes(),
		&csrfResponse,
	); err != nil {
		t.Fatal(err)
	}

	var preLoginCookie *http.Cookie

	for _, cookie := range csrfRec.Result().Cookies() {
		if cookie.Name == loginCSRFCookie {
			preLoginCookie = cookie
		}
	}

	if preLoginCookie == nil ||
		csrfResponse.Data.CSRF == "" {
		t.Fatal("login CSRF cookie/token missing")
	}

	login := func(
		password string,
		includeCSRF bool,
	) *httptest.ResponseRecorder {
		body := `{"username":"staff","password":"` +
			password + `"}`

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/auth/login",
			strings.NewReader(body),
		)

		req.Header.Set(
			"Content-Type",
			"application/json",
		)
		req.Header.Set(
			"Origin",
			"http://127.0.0.1:8080",
		)

		if includeCSRF {
			req.Header.Set(
				"X-CSRF-Token",
				csrfResponse.Data.CSRF,
			)
		}

		req.AddCookie(preLoginCookie)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		return rec
	}

	if rec := login("correct-password", false); rec.Code != 403 {
		t.Fatalf("missing CSRF: got %d", rec.Code)
	}

	if rec := login("bad-password", true); rec.Code != 401 {
		t.Fatalf("wrong password: got %d", rec.Code)
	}

	loginRec := login("correct-password", true)

	if loginRec.Code != 200 {
		t.Fatalf(
			"login failed: %d %s",
			loginRec.Code,
			loginRec.Body.String(),
		)
	}

	var sessionCookie *http.Cookie

	for _, cookie := range loginRec.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName {
			sessionCookie = cookie
		}
	}

	if sessionCookie == nil || !sessionCookie.HttpOnly {
		t.Fatal("valid session cookie missing")
	}

	meReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/auth/me",
		nil,
	)
	meReq.AddCookie(sessionCookie)

	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)

	if meRec.Code != 200 {
		t.Fatalf("/me returned %d", meRec.Code)
	}

	tokenReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/auth/csrf",
		nil,
	)
	tokenReq.AddCookie(sessionCookie)

	tokenRec := httptest.NewRecorder()
	router.ServeHTTP(tokenRec, tokenReq)

	if tokenRec.Code != 200 {
		t.Fatalf(
			"session csrf returned %d",
			tokenRec.Code,
		)
	}

	var tokenResponse struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}

	if err := json.Unmarshal(
		tokenRec.Body.Bytes(),
		&tokenResponse,
	); err != nil {
		t.Fatal(err)
	}

	logoutReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/logout",
		nil,
	)
	logoutReq.Header.Set(
		"Origin",
		"http://127.0.0.1:8080",
	)
	logoutReq.AddCookie(sessionCookie)

	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != 403 {
		t.Fatalf(
			"logout without CSRF: got %d",
			logoutRec.Code,
		)
	}

	logoutReq.Header.Set(
		"X-CSRF-Token",
		tokenResponse.Data.CSRF,
	)

	logoutRec = httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != 204 || !sessions.revoked {
		t.Fatalf(
			"logout failed: %d",
			logoutRec.Code,
		)
	}
}

func TestProtectRoleAndPasswordChange(t *testing.T) {
	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	sessions := &fakeSessions{
		role: "staff",
	}

	handler, err := NewAuthHandler(
		fakeAccountFinder{},
		sessions,
		logger,
		"http://127.0.0.1:8080",
		false,
	)

	if err != nil {
		t.Fatal(err)
	}

	protected := handler.Protect(
		"admin",
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/admin",
		nil,
	)
	req.AddCookie(&http.Cookie{
		Name:  auth.SessionCookieName,
		Value: strings.Repeat("a", 43),
	})

	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != 403 {
		t.Fatalf(
			"staff accessing admin route: %d",
			rec.Code,
		)
	}

	sessions.role = "admin"

	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Fatalf(
			"admin access denied: %d",
			rec.Code,
		)
	}

	sessions.mustChange = true

	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != 403 {
		t.Fatalf(
			"mandatory password change not enforced: %d",
			rec.Code,
		)
	}
}
