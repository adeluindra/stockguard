package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
)

type passwordChanger interface {
	ChangePassword(
		context.Context,
		int64,
		string,
		string,
	) error
}

// POST /api/v1/auth/change-password
// This endpoint must remain accessible when
// must_change_password is true.
func (h *AuthHandler) changePassword(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !h.sameOrigin(r) {
		h.error(
			w, r,
			http.StatusForbidden,
			"CSRF_INVALID",
			"Request forbidden",
		)
		return
	}

	identity, token, ok := h.currentIdentity(w, r)
	if !ok {
		return
	}

	if !h.sessionCSRFValid(r, token) {
		h.error(
			w, r,
			http.StatusForbidden,
			"CSRF_INVALID",
			"Request forbidden",
		)
		return
	}

	changer, ok := h.users.(passwordChanger)
	if !ok {
		h.logger.Error("password_changer_not_configured")

		h.error(
			w, r,
			http.StatusServiceUnavailable,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	contentType, _, err := mime.ParseMediaType(
		r.Header.Get("Content-Type"),
	)

	if err != nil || contentType != "application/json" {
		h.error(
			w, r,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"Expected application/json",
		)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var payload struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}

	if err := decoder.Decode(&payload); err != nil {
		h.error(
			w, r,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"Invalid JSON payload",
		)
		return
	}

	var trailing any

	if err := decoder.Decode(&trailing); err != io.EOF {
		h.error(
			w, r,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"Invalid JSON payload",
		)
		return
	}

	if len(payload.CurrentPassword) == 0 ||
		len(payload.CurrentPassword) > 1024 {
		h.error(
			w, r,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"Invalid current password input",
		)
		return
	}

	if err := auth.ValidateNewPassword(
		payload.NewPassword,
	); err != nil {
		h.error(
			w, r,
			http.StatusBadRequest,
			"PASSWORD_POLICY_VIOLATION",
			"New password must contain at least 16 characters",
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		15*time.Second,
	)
	defer cancel()

	err = changer.ChangePassword(
		ctx,
		identity.UserID,
		payload.CurrentPassword,
		payload.NewPassword,
	)

	switch {
	case errors.Is(
		err,
		auth.ErrCurrentPasswordIncorrect,
	):
		h.error(
			w, r,
			http.StatusForbidden,
			"CURRENT_PASSWORD_INVALID",
			"Current password is incorrect",
		)
		return

	case errors.Is(err, auth.ErrPasswordReused):
		h.error(
			w, r,
			http.StatusConflict,
			"PASSWORD_REUSED",
			"New password must differ from current password",
		)
		return

	case errors.Is(err, auth.ErrNewPasswordInvalid):
		h.error(
			w, r,
			http.StatusBadRequest,
			"PASSWORD_POLICY_VIOLATION",
			"New password does not meet password policy",
		)
		return

	case errors.Is(err, auth.ErrInactiveUser):
		h.error(
			w, r,
			http.StatusUnauthorized,
			"UNAUTHORIZED",
			"Authentication required",
		)
		return

	case err != nil:
		h.logger.Error(
			"change_password_failed",
			"error", err,
		)

		h.error(
			w, r,
			http.StatusServiceUnavailable,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	// The database has revoked all existing sessions.
	// Clear the caller's browser cookie as well.
	http.SetCookie(
		w,
		auth.ClearSessionCookie(h.secureCookies),
	)

	w.WriteHeader(http.StatusNoContent)
}
