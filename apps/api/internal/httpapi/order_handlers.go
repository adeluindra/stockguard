package httpapi

import (
	"context"
	"errors"
	"io"
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

	Transition(
		context.Context,
		int64,
		int64,
		orders.TransitionAction,
	) (orders.TransitionResult, error)
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

	mux.Handle(
		"POST /api/v1/orders",
		authHandler.Protect(
			"",
			http.HandlerFunc(h.create),
		),
	)

	mux.Handle(
		"POST /api/v1/orders/{id}/fulfill",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.fulfill),
		),
	)

	mux.Handle(
		"POST /api/v1/orders/{id}/cancel",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.cancel),
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
			w, r, 400,
			"INVALID_ORDER",
			"Invalid order request",
		)

	case errors.Is(err, orders.ErrIdempotencyConflict):
		h.auth.error(
			w, r, 409,
			"IDEMPOTENCY_CONFLICT",
			"Idempotency key was used with a different order",
		)

	case errors.Is(err, orders.ErrProductNotFound):
		h.auth.error(
			w, r, 404,
			"PRODUCT_NOT_FOUND",
			"Order contains a product that does not exist",
		)

	case errors.Is(err, orders.ErrProductInactive):
		h.auth.error(
			w, r, 409,
			"PRODUCT_INACTIVE",
			"Order contains an inactive product",
		)

	case errors.Is(err, orders.ErrInsufficientStock):
		h.auth.error(
			w, r, 409,
			"INSUFFICIENT_STOCK",
			"Insufficient available stock for one or more items",
		)

	default:
		h.auth.logger.Error(
			"create_order_failed",
			"error", err,
		)

		h.auth.error(
			w, r, 503,
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
			w, r, 401,
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

	key, err := orders.NormalizeIdempotencyKey(keys[0])
	if err != nil {
		h.failure(w, r, err)
		return
	}

	var payload struct {
		Items []orders.Item `json:"items"`
	}

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

func parseOrderID(r *http.Request) (int64, error) {
	raw := r.PathValue("id")

	id, err := strconv.ParseInt(raw, 10, 64)

	if err != nil ||
		id <= 0 ||
		strconv.FormatInt(id, 10) != raw {
		return 0, orders.ErrInvalidTransition
	}

	return id, nil
}

func (h *orderHandler) transitionFailure(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, orders.ErrInvalidTransition):
		h.auth.error(
			w, r, 400,
			"INVALID_TRANSITION",
			"Invalid order transition request",
		)

	case errors.Is(err, orders.ErrOrderNotFound):
		h.auth.error(
			w, r, 404,
			"ORDER_NOT_FOUND",
			"Order not found",
		)

	case errors.Is(err, orders.ErrOrderNotReserved):
		h.auth.error(
			w, r, 409,
			"ORDER_ALREADY_FINALIZED",
			"Order cannot change terminal status",
		)

	default:
		h.auth.logger.Error(
			"order_transition_failed",
			"error", err,
		)

		h.auth.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
	}
}

func (h *orderHandler) fulfill(
	w http.ResponseWriter,
	r *http.Request,
) {
	h.transition(w, r, orders.ActionFulfill)
}

func (h *orderHandler) cancel(
	w http.ResponseWriter,
	r *http.Request,
) {
	h.transition(w, r, orders.ActionCancel)
}

func (h *orderHandler) transition(
	w http.ResponseWriter,
	r *http.Request,
	action orders.TransitionAction,
) {
	id, err := parseOrderID(r)
	if err != nil {
		h.transitionFailure(w, r, err)
		return
	}

	// These endpoints accept an empty body only.
	r.Body = http.MaxBytesReader(w, r.Body, 1)

	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) != 0 {
		h.transitionFailure(
			w, r, orders.ErrInvalidTransition,
		)
		return
	}

	identity, ok := IdentityFromContext(r.Context())
	if !ok || identity.UserID <= 0 {
		h.auth.error(
			w, r, 401,
			"UNAUTHORIZED",
			"Authentication required",
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		15*time.Second,
	)
	defer cancel()

	result, err := h.store.Transition(
		ctx,
		identity.UserID,
		id,
		action,
	)

	if err != nil {
		h.transitionFailure(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     result.Order,
		"replayed": result.Replayed,
	})
}
