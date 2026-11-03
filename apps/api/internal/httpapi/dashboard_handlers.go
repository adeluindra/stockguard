package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/dashboard"
)

type DashboardService interface {
	GetSummary(context.Context) (dashboard.Summary, error)

	ListLowStock(
		context.Context,
		int,
		int,
	) (dashboard.Page[dashboard.LowStockProduct], error)

	ListActivity(
		context.Context,
		int,
		int,
	) (dashboard.Page[dashboard.Activity], error)
}

type dashboardHandler struct {
	auth  *AuthHandler
	store DashboardService
}

func RegisterDashboardRoutes(
	mux *http.ServeMux,
	authHandler *AuthHandler,
	store DashboardService,
) {
	h := &dashboardHandler{
		auth:  authHandler,
		store: store,
	}

	mux.Handle(
		"GET /api/v1/dashboard/summary",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.summary),
		),
	)

	mux.Handle(
		"GET /api/v1/dashboard/low-stock",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.lowStock),
		),
	)

	mux.Handle(
		"GET /api/v1/dashboard/activity",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.activity),
		),
	)
}

func parseDashboardPage(
	r *http.Request,
) (int, int, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, 0, dashboard.ErrInvalidPage
	}

	limit := 20
	offset := 0

	for key, values := range query {
		if key != "limit" && key != "offset" {
			return 0, 0, dashboard.ErrInvalidPage
		}

		if len(values) != 1 || values[0] == "" {
			return 0, 0, dashboard.ErrInvalidPage
		}

		value, err := strconv.Atoi(values[0])
		if err != nil {
			return 0, 0, dashboard.ErrInvalidPage
		}

		switch key {
		case "limit":
			limit = value
		case "offset":
			offset = value
		}
	}

	if err := dashboard.ValidatePage(
		limit, offset,
	); err != nil {
		return 0, 0, err
	}

	return limit, offset, nil
}

func (h *dashboardHandler) failure(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	if errors.Is(err, dashboard.ErrInvalidPage) {
		h.auth.error(
			w,
			r,
			http.StatusBadRequest,
			"INVALID_DASHBOARD_QUERY",
			"Invalid dashboard query",
		)
		return
	}

	h.auth.logger.Error(
		"dashboard_query_failed",
		"error", err,
	)

	h.auth.error(
		w,
		r,
		http.StatusServiceUnavailable,
		"SERVICE_UNAVAILABLE",
		"Service unavailable",
	)
}

func (h *dashboardHandler) summary(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.URL.RawQuery != "" {
		h.failure(w, r, dashboard.ErrInvalidPage)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		5*time.Second,
	)
	defer cancel()

	summary, err := h.store.GetSummary(ctx)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": summary,
	})
}

func (h *dashboardHandler) lowStock(
	w http.ResponseWriter,
	r *http.Request,
) {
	limit, offset, err := parseDashboardPage(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		5*time.Second,
	)
	defer cancel()

	page, err := h.store.ListLowStock(
		ctx,
		limit,
		offset,
	)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": page.Items,
		"pagination": map[string]any{
			"limit":    page.Limit,
			"offset":   page.Offset,
			"has_more": page.HasMore,
		},
	})
}

func (h *dashboardHandler) activity(
	w http.ResponseWriter,
	r *http.Request,
) {
	limit, offset, err := parseDashboardPage(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		5*time.Second,
	)
	defer cancel()

	page, err := h.store.ListActivity(
		ctx,
		limit,
		offset,
	)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": page.Items,
		"pagination": map[string]any{
			"limit":    page.Limit,
			"offset":   page.Offset,
			"has_more": page.HasMore,
		},
	})
}
