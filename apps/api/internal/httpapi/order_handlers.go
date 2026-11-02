package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/orders"
)

type OrderService interface {
	Create(
		context.Context,
		int64,
		string,
		[]orders.Item,
	) (orders.CreateResult, error)
}

type orderHandler struct {
	auth  *AuthHandler
	store OrderService
}

func RegisterOrderRoutes(
	mux *http.ServeMux,
	authHandler *AuthHandler,
	store OrderService,
) {
	h := &orderHandler{
		auth:  authHandler,
		store: store,
	}

	// Both Admin and Staff may create an order.
	// Protect enforces authentication, password-change
	// requirements, same-origin and session CSRF.
	mux.Handle(
		"POST /api/v1/orders",
		authHandler.Protect(
			"",
			http.HandlerFunc(h.create),
		),
	)
}

func (h *orderHandler) failure(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, orders.ErrInvalidOrder):
		h.auth.error(
			w, r,
			http.StatusBadRequest,
			"INVALID_ORDER",
			"Invalid order request",
		)

	case errors.Is(err, orders.ErrIdempotencyConflict):
		h.auth.error(
			w, r,
			http.StatusConflict,
			"IDEMPOTENCY_CONFLICT",
			"Idempotency key was used with a different order",
		)

	case errors.Is(err, orders.ErrProductNotFound):
		h.auth.error(
			w, r,
			http.StatusNotFound,
			"PRODUCT_NOT_FOUND",
			"Order contains a product that does not exist",
		)

	case errors.Is(err, orders.ErrProductInactive):
		h.auth.error(
			w, r,
			http.StatusConflict,
			"PRODUCT_INACTIVE",
			"Order contains an inactive product",
		)

	case errors.Is(err, orders.ErrInsufficientStock):
		h.auth.error(
			w, r,
			http.StatusConflict,
			"INSUFFICIENT_STOCK",
			"Insufficient available stock for one or more items",
		)

	default:
		h.auth.logger.Error(
			"create_order_failed",
			"error", err,
		)

		h.auth.error(
			w, r,
			http.StatusServiceUnavailable,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
	}
}

func (h *orderHandler) create(
	w http.ResponseWriter,
	r *http.Request,
) {
	identity, ok := IdentityFromContext(r.Context())

	if !ok || identity.UserID <= 0 {
		h.auth.error(
			w, r,
			http.StatusUnauthorized,
			"UNAUTHORIZED",
			"Authentication required",
		)
		return
	}

	keys := r.Header.Values("Idempotency-Key")

	if len(keys) != 1 {
		h.failure(w, r, orders.ErrInvalidOrder)
		return
	}

	key, err := orders.NormalizeIdempotencyKey(
		keys[0],
	)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	var payload struct {
		Items []orders.Item `json:"items"`
	}

	// Reuse BE-3's strict JSON decoder:
	// content-type, max body length, unknown fields,
	// and trailing JSON are all validated.
	if err := decodeProductJSON(
		w, r, &payload,
	); err != nil {
		h.failure(w, r, orders.ErrInvalidOrder)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		15*time.Second,
	)
	defer cancel()

	result, err := h.store.Create(
		ctx,
		identity.UserID,
		key,
		payload.Items,
	)

	if err != nil {
		h.failure(w, r, err)
		return
	}

	w.Header().Set(
		"Location",
		"/api/v1/orders/"+strconv.FormatInt(
			result.Order.ID, 10,
		),
	)

	status := http.StatusCreated

	if result.Replayed {
		status = http.StatusOK
	}

	writeJSON(w, status, map[string]any{
		"data":     result.Order,
		"replayed": result.Replayed,
	})
}
