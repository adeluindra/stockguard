package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
)

type staffManager interface {
	ListStaff(
		context.Context,
	) ([]auth.StaffUser, error)

	CreateStaff(
		context.Context,
		string,
	) (auth.StaffUser, string, error)

	ResetStaffPassword(
		context.Context,
		int64,
	) (string, error)

	DeactivateStaff(
		context.Context,
		int64,
	) error
}

func (h *AuthHandler) RegisterStaffRoutes(
	mux *http.ServeMux,
) {
	mux.Handle(
		"GET /api/v1/admin/staff",
		h.Protect("admin", http.HandlerFunc(h.listStaff)),
	)

	mux.Handle(
		"POST /api/v1/admin/staff",
		h.Protect("admin", http.HandlerFunc(h.createStaff)),
	)

	mux.Handle(
		"POST /api/v1/admin/staff/{id}/reset-password",
		h.Protect("admin", http.HandlerFunc(h.resetStaffPassword)),
	)

	mux.Handle(
		"POST /api/v1/admin/staff/{id}/deactivate",
		h.Protect("admin", http.HandlerFunc(h.deactivateStaff)),
	)
}

func (h *AuthHandler) staffStore(
	w http.ResponseWriter,
	r *http.Request,
) (staffManager, bool) {
	store, ok := h.users.(staffManager)

	if !ok {
		h.logger.Error("staff_manager_not_configured")
		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return nil, false
	}

	return store, true
}

func (h *AuthHandler) listStaff(
	w http.ResponseWriter,
	r *http.Request,
) {
	store, ok := h.staffStore(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(), 5*time.Second,
	)
	defer cancel()

	users, err := store.ListStaff(ctx)
	if err != nil {
		h.logger.Error("list_staff_failed", "error", err)
		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	writeJSON(w, 200, map[string]any{
		"data": users,
	})
}

func (h *AuthHandler) createStaff(
	w http.ResponseWriter,
	r *http.Request,
) {
	store, ok := h.staffStore(w, r)
	if !ok {
		return
	}

	contentType, _, err := mime.ParseMediaType(
		r.Header.Get("Content-Type"),
	)

	if err != nil || contentType != "application/json" {
		h.error(
			w, r, 400,
			"INVALID_REQUEST",
			"Expected application/json",
		)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 2048)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request struct {
		Username string `json:"username"`
	}

	if err := decoder.Decode(&request); err != nil {
		h.error(
			w, r, 400,
			"INVALID_REQUEST",
			"Invalid JSON request",
		)
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		h.error(
			w, r, 400,
			"INVALID_REQUEST",
			"Unexpected JSON content",
		)
		return
	}

	if err := auth.ValidateStaffUsername(
		request.Username,
	); err != nil {
		h.error(
			w, r, 400,
			"INVALID_USERNAME",
			"Username must contain 3-50 letters, digits, _ or -",
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(), 10*time.Second,
	)
	defer cancel()

	user, temp, err := store.CreateStaff(
		ctx,
		request.Username,
	)

	switch {
	case errors.Is(err, auth.ErrUsernameTaken):
		h.error(
			w, r, 409,
			"USERNAME_TAKEN",
			"Username already exists",
		)
		return

	case errors.Is(err, auth.ErrInvalidStaffUsername):
		h.error(
			w, r, 400,
			"INVALID_USERNAME",
			"Invalid username",
		)
		return

	case err != nil:
		h.logger.Error("create_staff_failed", "error", err)
		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"user":               user,
			"temporary_password": temp,
		},
	})
}

func parseStaffID(
	r *http.Request,
) (int64, error) {
	value := r.PathValue("id")

	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil ||
		id <= 0 ||
		strconv.FormatInt(id, 10) != value {
		return 0, errors.New("invalid staff ID")
	}

	return id, nil
}

func (h *AuthHandler) resetStaffPassword(
	w http.ResponseWriter,
	r *http.Request,
) {
	store, ok := h.staffStore(w, r)
	if !ok {
		return
	}

	id, err := parseStaffID(r)
	if err != nil {
		h.error(
			w, r, 400,
			"INVALID_STAFF_ID",
			"Invalid staff ID",
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(), 10*time.Second,
	)
	defer cancel()

	temp, err := store.ResetStaffPassword(ctx, id)

	if errors.Is(err, auth.ErrStaffNotFound) {
		h.error(
			w, r, 404,
			"STAFF_NOT_FOUND",
			"Active staff not found",
		)
		return
	}

	if err != nil {
		h.logger.Error(
			"reset_staff_password_failed",
			"error", err,
		)
		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	writeJSON(w, 200, map[string]any{
		"data": map[string]any{
			"user_id":              id,
			"temporary_password":   temp,
			"must_change_password": true,
		},
	})
}

func (h *AuthHandler) deactivateStaff(
	w http.ResponseWriter,
	r *http.Request,
) {
	store, ok := h.staffStore(w, r)
	if !ok {
		return
	}

	id, err := parseStaffID(r)
	if err != nil {
		h.error(
			w, r, 400,
			"INVALID_STAFF_ID",
			"Invalid staff ID",
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(), 10*time.Second,
	)
	defer cancel()

	err = store.DeactivateStaff(ctx, id)

	if errors.Is(err, auth.ErrStaffNotFound) {
		h.error(
			w, r, 404,
			"STAFF_NOT_FOUND",
			"Active staff not found",
		)
		return
	}

	if err != nil {
		h.logger.Error(
			"deactivate_staff_failed",
			"error", err,
		)
		h.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
