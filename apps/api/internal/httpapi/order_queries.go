package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/orders"
)

type OrderQueryService interface {
	List(
		context.Context,
		int64,
		bool,
		string,
		int,
		int,
	) (orders.OrderPage, error)

	Get(
		context.Context,
		int64,
		bool,
		int64,
	) (orders.Order, error)
}

func (h *orderHandler) queryService(
	w http.ResponseWriter,
	r *http.Request,
) (OrderQueryService, bool) {
	reader, ok := h.store.(OrderQueryService)
	if !ok {
		h.auth.logger.Error("order_query_service_not_configured")
		h.auth.error(
			w, r, http.StatusServiceUnavailable,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
		return nil, false
	}

	return reader, true
}

func (h *orderHandler) queryFailure(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, orders.ErrInvalidOrderQuery):
		h.auth.error(
			w, r, http.StatusBadRequest,
			"INVALID_ORDER_QUERY",
			"Invalid order query",
		)

	case errors.Is(err, orders.ErrOrderNotFound):
		h.auth.error(
			w, r, http.StatusNotFound,
			"ORDER_NOT_FOUND",
			"Order not found",
		)

	default:
		h.auth.logger.Error(
			"order_query_failed",
			"error", err,
		)

		h.auth.error(
			w, r, http.StatusServiceUnavailable,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
	}
}

func (h *orderHandler) listOrders(
	w http.ResponseWriter,
	r *http.Request,
) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok || identity.UserID <= 0 {
		h.auth.error(
			w, r, http.StatusUnauthorized,
			"UNAUTHORIZED",
			"Authentication required",
		)
		return
	}

	query := r.URL.Query()
	for key, values := range query {
		switch key {
		case "status", "limit", "offset":
			if len(values) != 1 || values[0] == "" {
				h.queryFailure(
					w, r, orders.ErrInvalidOrderQuery,
				)
				return
			}
		default:
			h.queryFailure(
				w, r, orders.ErrInvalidOrderQuery,
			)
			return
		}
	}

	status := query.Get("status")
	limit := 20
	offset := 0

	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			h.queryFailure(
				w, r, orders.ErrInvalidOrderQuery,
			)
			return
		}
		limit = value
	}

	if raw := query.Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			h.queryFailure(
				w, r, orders.ErrInvalidOrderQuery,
			)
			return
		}
		offset = value
	}

	if err := orders.ValidateOrderQuery(
		status, limit, offset,
	); err != nil {
		h.queryFailure(w, r, err)
		return
	}

	reader, ok := h.queryService(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(), 5*time.Second,
	)
	defer cancel()

	page, err := reader.List(
		ctx,
		identity.UserID,
		identity.Role == "admin",
		status,
		limit,
		offset,
	)
	if err != nil {
		h.queryFailure(w, r, err)
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

func (h *orderHandler) getOrder(
	w http.ResponseWriter,
	r *http.Request,
) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok || identity.UserID <= 0 {
		h.auth.error(
			w, r, http.StatusUnauthorized,
			"UNAUTHORIZED",
			"Authentication required",
		)
		return
	}

	if r.URL.RawQuery != "" {
		h.queryFailure(
			w, r, orders.ErrInvalidOrderQuery,
		)
		return
	}

	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)

	if err != nil ||
		id <= 0 ||
		strconv.FormatInt(id, 10) != raw {
		h.queryFailure(
			w, r, orders.ErrInvalidOrderQuery,
		)
		return
	}

	reader, ok := h.queryService(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(), 5*time.Second,
	)
	defer cancel()

	order, err := reader.Get(
		ctx,
		identity.UserID,
		identity.Role == "admin",
		id,
	)
	if err != nil {
		h.queryFailure(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": order,
	})
}
