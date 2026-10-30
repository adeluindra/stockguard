package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
)

const (
	loginCSRFCookie = "stockguard_login_csrf"
	loginCSRFMaxAge = 600
)

type AccountFinder interface {
	FindActiveByUsername(
		context.Context,
		string,
	) (auth.LoginAccount, error)
}

type SessionManager interface {
	Create(
		context.Context,
		int64,
	) (string, time.Time, error)

	Authenticate(
		context.Context,
		string,
	) (auth.SessionIdentity, error)

	Revoke(context.Context, string) error
}

// Local-development login throttling.
type loginWindow struct {
	count int
	until time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginWindow
}

func (l *loginLimiter) allow(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	for key, item := range l.attempts {
		if !now.Before(item.until) {
			delete(l.attempts, key)
		}
	}

	item := l.attempts[host]

	if !now.Before(item.until) {
		item = loginWindow{
			until: now.Add(15 * time.Minute),
		}
	}

	if item.count >= 10 {
		return false
	}

	if len(l.attempts) >= 2048 {
		if _, exists := l.attempts[host]; !exists {
			return false
		}
	}

	item.count++
	l.attempts[host] = item

	return true
}

type AuthHandler struct {
	users         AccountFinder
	sessions      SessionManager
	logger        *slog.Logger
	origin        string
	secureCookies bool
	dummyHash     string
	limiter       loginLimiter
}

func NewAuthHandler(
	users AccountFinder,
	sessions SessionManager,
	logger *slog.Logger,
	origin string,
	secureCookies bool,
) (*AuthHandler, error) {
	parsed, err := url.Parse(origin)

	if err != nil ||
		parsed.Host == "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" ||
		parsed.Path != "" ||
		(parsed.Scheme != "http" &&
			parsed.Scheme != "https") {
		return nil, errors.New(
			"APP_ORIGIN must be a valid HTTP(S) origin",
		)
	}

	if parsed.Scheme == "https" && !secureCookies {
		return nil, errors.New(
			"HTTPS requires SESSION_COOKIE_SECURE=true",
		)
	}

	if parsed.Scheme == "http" &&
		parsed.Hostname() != "127.0.0.1" &&
		parsed.Hostname() != "localhost" {
		return nil, errors.New(
			"HTTP origin allowed only on localhost",
		)
	}

	// Avoid skipping password verification timing
	// when the username does not exist.
	dummyHash, err := auth.HashPassword(
		"stockguard-invalid-account-placeholder",
	)
	if err != nil {
		return nil, err
	}

	return &AuthHandler{
		users:         users,
		sessions:      sessions,
		logger:        logger,
		origin:        origin,
		secureCookies: secureCookies,
		dummyHash:     dummyHash,
		limiter: loginLimiter{
			attempts: make(map[string]loginWindow),
		},
	}, nil
}

func (h *AuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc(
		"GET /api/v1/auth/login-csrf",
		h.loginCSRF,
	)

	mux.HandleFunc(
		"POST /api/v1/auth/login",
		h.login,
	)

	mux.HandleFunc(
		"GET /api/v1/auth/me",
		h.me,
	)

	mux.HandleFunc(
		"GET /api/v1/auth/csrf",
		h.sessionCSRF,
	)

	mux.HandleFunc(
		"POST /api/v1/auth/logout",
		h.logout,
	)

	mux.HandleFunc(
		"POST /api/v1/auth/change-password",
		h.changePassword,
	)
}

func (h *AuthHandler) error(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code string,
	message string,
) {
	requestID, _ := r.Context().
		Value(requestIDKey{}).(string)

	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"details":    []any{},
			"request_id": requestID,
		},
	})
}

// Require a same-origin browser request for unsafe actions.
func (h *AuthHandler) sameOrigin(
	r *http.Request,
) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin == h.origin
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		return false
	}

	parsed, err := url.Parse(referer)

	return err == nil &&
		parsed.Scheme+"://"+parsed.Host == h.origin &&
		parsed.User == nil
}

func newCSRFNonce() (string, error) {
	var raw [32]byte

	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(
		raw[:],
	), nil
}

// GET /api/v1/auth/login-csrf
func (h *AuthHandler) loginCSRF(
	w http.ResponseWriter,
	r *http.Request,
) {
	token, err := newCSRFNonce()

	if err != nil {
		h.logger.Error(
			"csrf_rng_failed",
			"error", err,
		)
		h.error(
			w, r, 500,
			"INTERNAL_ERROR",
			"Internal server error",
		)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     loginCSRFCookie,
		Value:    token,
		Path:     "/api/v1/auth",
		MaxAge:   loginCSRFMaxAge,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, 200, map[string]any{
		"data": map[string]string{
			"csrf_token": token,
		},
	})
}

func safeCompare(a, b string) bool {
	if len(a) != 43 || len(b) != 43 {
		return false
	}

	return subtle.ConstantTimeCompare(
		[]byte(a),
		[]byte(b),
	) == 1
}

func (h *AuthHandler) loginCSRFValid(
	r *http.Request,
) bool {
	cookie, err := r.Cookie(loginCSRFCookie)

	return err == nil &&
		safeCompare(
			cookie.Value,
			r.Header.Get("X-CSRF-Token"),
		)
}

func (h *AuthHandler) clearLoginCSRF(
	w http.ResponseWriter,
) {
	http.SetCookie(w, &http.Cookie{
		Name:     loginCSRFCookie,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func readLoginRequest(
	w http.ResponseWriter,
	r *http.Request,
) (string, string, error) {
	contentType, _, err := mime.ParseMediaType(
		r.Header.Get("Content-Type"),
	)

	if err != nil || contentType != "application/json" {
		return "", "", errors.New(
			"expected application/json",
		)
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := decoder.Decode(&payload); err != nil {
		return "", "", err
	}

	var trailing any

	if err := decoder.Decode(&trailing); err != io.EOF {
		return "", "", errors.New(
			"unexpected trailing JSON",
		)
	}

	if len(payload.Username) < 3 ||
		len(payload.Username) > 200 ||
		len(payload.Password) == 0 ||
		len(payload.Password) > 1024 {
		return "", "", errors.New("invalid input")
	}

	return payload.Username, payload.Password, nil
}

// POST /api/v1/auth/login
func (h *AuthHandler) login(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !h.sameOrigin(r) || !h.loginCSRFValid(r) {
		h.error(
			w, r, 403,
			"CSRF_INVALID",
			"Request forbidden",
		)
		return
	}

	username, password, err := readLoginRequest(w, r)

	if err != nil {
		h.error(
			w, r, 400,
			"INVALID_REQUEST",
			"Invalid login request",
		)
		return
	}

	if !h.limiter.allow(r.RemoteAddr) {
		h.error(
			w, r, 429,
			"TOO_MANY_REQUESTS",
			"Too many login attempts",
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		5*time.Second,
	)
	defer cancel()

	user, findErr := h.users.FindActiveByUsername(
		ctx,
		username,
	)

	if findErr != nil &&
		!errors.Is(findErr, sql.ErrNoRows) {
		h.logger.Error(
			"login_lookup_failed",
			"error", findErr,
		)

		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	encodedHash := h.dummyHash

	if findErr == nil {
		encodedHash = user.PasswordHash
	}

	valid, verifyErr := auth.VerifyPassword(
		password,
		encodedHash,
	)

	if verifyErr != nil {
		h.logger.Error(
			"login_password_hash_invalid",
			"error", verifyErr,
		)

		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	if findErr != nil || !valid {
		h.error(
			w, r, 401,
			"INVALID_CREDENTIALS",
			"Invalid username or password",
		)
		return
	}

	token, expiresAt, err := h.sessions.Create(
		ctx,
		user.ID,
	)

	if errors.Is(err, auth.ErrInactiveUser) {
		h.error(
			w, r, 401,
			"INVALID_CREDENTIALS",
			"Invalid username or password",
		)
		return
	}

	if err != nil {
		h.logger.Error(
			"session_create_failed",
			"error", err,
		)

		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	http.SetCookie(
		w,
		auth.SessionCookie(
			token,
			expiresAt,
			h.secureCookies,
		),
	)

	h.clearLoginCSRF(w)

	writeJSON(w, 200, map[string]any{
		"data": map[string]any{
			"id":                   user.ID,
			"username":             user.Username,
			"role":                 user.Role,
			"must_change_password": user.MustChangePassword,
		},
	})
}

// CSRF token is bound to the random session token.
// The session token itself remains HttpOnly.
func csrfFromSessionToken(token string) string {
	mac := hmac.New(
		sha256.New,
		[]byte(token),
	)

	_, _ = mac.Write(
		[]byte("stockguard:csrf:v1"),
	)

	return base64.RawURLEncoding.EncodeToString(
		mac.Sum(nil),
	)
}

func (h *AuthHandler) sessionToken(
	r *http.Request,
) (string, error) {
	cookie, err := r.Cookie(auth.SessionCookieName)

	if err != nil || cookie.Value == "" {
		return "", auth.ErrInvalidSession
	}

	return cookie.Value, nil
}

func (h *AuthHandler) currentIdentity(
	w http.ResponseWriter,
	r *http.Request,
) (auth.SessionIdentity, string, bool) {
	token, err := h.sessionToken(r)

	if err != nil {
		h.error(
			w, r, 401,
			"UNAUTHORIZED",
			"Authentication required",
		)
		return auth.SessionIdentity{}, "", false
	}

	identity, err := h.sessions.Authenticate(
		r.Context(),
		token,
	)

	if errors.Is(err, auth.ErrInvalidSession) {
		h.error(
			w, r, 401,
			"UNAUTHORIZED",
			"Authentication required",
		)
		return auth.SessionIdentity{}, "", false
	}

	if err != nil {
		h.logger.Error(
			"session_auth_failed",
			"error", err,
		)

		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return auth.SessionIdentity{}, "", false
	}

	return identity, token, true
}

// GET /api/v1/auth/me
func (h *AuthHandler) me(
	w http.ResponseWriter,
	r *http.Request,
) {
	identity, _, ok := h.currentIdentity(w, r)
	if !ok {
		return
	}

	writeJSON(w, 200, map[string]any{
		"data": map[string]any{
			"user_id":              identity.UserID,
			"role":                 identity.Role,
			"must_change_password": identity.MustChangePassword,
			"expires_at":           identity.ExpiresAt,
		},
	})
}

// GET /api/v1/auth/csrf
func (h *AuthHandler) sessionCSRF(
	w http.ResponseWriter,
	r *http.Request,
) {
	_, token, ok := h.currentIdentity(w, r)
	if !ok {
		return
	}

	writeJSON(w, 200, map[string]any{
		"data": map[string]string{
			"csrf_token": csrfFromSessionToken(token),
		},
	})
}

func (h *AuthHandler) sessionCSRFValid(
	r *http.Request,
	token string,
) bool {
	return safeCompare(
		csrfFromSessionToken(token),
		r.Header.Get("X-CSRF-Token"),
	)
}

// POST /api/v1/auth/logout
func (h *AuthHandler) logout(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !h.sameOrigin(r) {
		h.error(
			w, r, 403,
			"CSRF_INVALID",
			"Request forbidden",
		)
		return
	}

	token, err := h.sessionToken(r)

	if err != nil {
		http.SetCookie(
			w,
			auth.ClearSessionCookie(h.secureCookies),
		)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if !h.sessionCSRFValid(r, token) {
		h.error(
			w, r, 403,
			"CSRF_INVALID",
			"Request forbidden",
		)
		return
	}

	err = h.sessions.Revoke(r.Context(), token)

	if err != nil &&
		!errors.Is(err, auth.ErrInvalidSession) {
		h.logger.Error(
			"session_revoke_failed",
			"error", err,
		)

		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	http.SetCookie(
		w,
		auth.ClearSessionCookie(h.secureCookies),
	)

	w.WriteHeader(http.StatusNoContent)
}

type identityContextKey struct{}

// Allows business handlers to access the authenticated user.
func IdentityFromContext(
	ctx context.Context,
) (auth.SessionIdentity, bool) {
	identity, ok := ctx.Value(
		identityContextKey{},
	).(auth.SessionIdentity)

	return identity, ok
}

// Protect applies authentication, authorization, and CSRF
// protection to future business routes.
func (h *AuthHandler) Protect(
	role string,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		identity, token, ok := h.currentIdentity(w, r)
		if !ok {
			return
		}

		if identity.MustChangePassword {
			h.error(
				w, r, 403,
				"PASSWORD_CHANGE_REQUIRED",
				"Password change required",
			)
			return
		}

		if role != "" && identity.Role != role {
			h.error(
				w, r, 403,
				"FORBIDDEN",
				"Insufficient permissions",
			)
			return
		}

		unsafe := r.Method != http.MethodGet &&
			r.Method != http.MethodHead &&
			r.Method != http.MethodOptions

		if unsafe {
			if !h.sameOrigin(r) ||
				!h.sessionCSRFValid(r, token) {
				h.error(
					w, r, 403,
					"CSRF_INVALID",
					"Request forbidden",
				)
				return
			}
		}

		ctx := context.WithValue(
			r.Context(),
			identityContextKey{},
			identity,
		)

		next.ServeHTTP(
			w,
			r.WithContext(ctx),
		)
	})
}
