package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/adeluindra/stockguard/apps/api/internal/inventory"
	"github.com/adeluindra/stockguard/apps/api/internal/products"
)

type InventoryService interface {
	GetBalance(
		context.Context,
		int64,
	) (inventory.Balance, error)

	Adjust(
		context.Context,
		int64,
		int64,
		int64,
		string,
	) (inventory.AdjustmentResult, error)

	ListMovements(
		context.Context,
		int64,
		int,
		int,
	) (inventory.MovementPage, error)
}

type inventoryHandler struct {
	auth  *AuthHandler
	store InventoryService
}

func RegisterInventoryRoutes(
	mux *http.ServeMux,
	authHandler *AuthHandler,
	store InventoryService,
) {
	h := &inventoryHandler{
		auth:  authHandler,
		store: store,
	}

	mux.Handle(
		"GET /api/v1/products/{id}/inventory",
		authHandler.Protect(
			"",
			http.HandlerFunc(h.balance),
		),
	)

	mux.Handle(
		"POST /api/v1/products/{id}/inventory/adjustments",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.adjust),
		),
	)

	mux.Handle(
		"GET /api/v1/products/{id}/inventory/movements",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.movements),
		),
	)
}

func inventoryProductID(
	r *http.Request,
) (int64, error) {
	raw := r.PathValue("id")

	id, err := strconv.ParseInt(raw, 10, 64)

	if err != nil ||
		id <= 0 ||
		strconv.FormatInt(id, 10) != raw {
		return 0, inventory.ErrInvalidAdjustment
	}

	return id, nil
}

func (h *inventoryHandler) failure(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, inventory.ErrInvalidAdjustment),
		errors.Is(err, inventory.ErrInvalidPagination),
		errors.Is(err, products.ErrInvalidInput):

		h.auth.error(
			w, r,
			http.StatusBadRequest,
			"INVALID_INVENTORY_REQUEST",
			"Invalid inventory request",
		)

	case errors.Is(err, inventory.ErrProductNotFound):
		h.auth.error(
			w, r,
			http.StatusNotFound,
			"PRODUCT_NOT_FOUND",
			"Product not found",
		)

	case errors.Is(err, inventory.ErrProductInactive):
		h.auth.error(
			w, r,
			http.StatusConflict,
			"PRODUCT_INACTIVE",
			"Product is inactive",
		)

	case errors.Is(err, inventory.ErrInsufficientAvailable):
		h.auth.error(
			w, r,
			http.StatusConflict,
			"INVENTORY_CONFLICT",
			"Adjustment exceeds available stock",
		)

	case errors.Is(err, inventory.ErrQuantityOverflow):
		h.auth.error(
			w, r,
			http.StatusConflict,
			"INVENTORY_OVERFLOW",
			"Inventory quantity limit exceeded",
		)

	default:
		h.auth.logger.Error(
			"inventory_operation_failed",
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

func (h *inventoryHandler) balance(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := inventoryProductID(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		5*time.Second,
	)
	defer cancel()

	b, err := h.store.GetBalance(ctx, id)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	identity, _ := IdentityFromContext(r.Context())

	if !b.IsActive && identity.Role != "admin" {
		h.failure(w, r, inventory.ErrProductNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": b,
	})
}

func (h *inventoryHandler) adjust(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := inventoryProductID(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	var payload struct {
		Delta  *int64 `json:"delta"`
		Reason string `json:"reason"`
	}

	if err := decodeProductJSON(
		w, r, &payload,
	); err != nil || payload.Delta == nil {
		h.failure(
			w, r,
			inventory.ErrInvalidAdjustment,
		)
		return
	}

	if err := inventory.ValidateAdjustment(
		*payload.Delta,
		payload.Reason,
	); err != nil {
		h.failure(w, r, err)
		return
	}

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

	ctx, cancel := context.WithTimeout(
		r.Context(),
		10*time.Second,
	)
	defer cancel()

	result, err := h.store.Adjust(
		ctx,
		id,
		identity.UserID,
		*payload.Delta,
		payload.Reason,
	)

	if err != nil {
		h.failure(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": result,
	})
}

func (h *inventoryHandler) movements(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := inventoryProductID(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	query := r.URL.Query()

	for key, values := range query {
		if (key != "limit" && key != "offset") ||
			len(values) != 1 {
			h.failure(
				w, r,
				inventory.ErrInvalidPagination,
			)
			return
		}
	}

	limit := 20
	offset := 0

	if value := query.Get("limit"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 100 {
			h.failure(
				w, r,
				inventory.ErrInvalidPagination,
			)
			return
		}
		limit = n
	}

	if value := query.Get("offset"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 1000000 {
			h.failure(
				w, r,
				inventory.ErrInvalidPagination,
			)
			return
		}
		offset = n
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		5*time.Second,
	)
	defer cancel()

	page, err := h.store.ListMovements(
		ctx, id, limit, offset,
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
