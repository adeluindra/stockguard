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

	"github.com/adeluindra/stockguard/apps/api/internal/products"
)

type ProductService interface {
	Get(context.Context, int64) (products.Product, error)
	List(context.Context, int, int, bool) (products.Page, error)
	Create(context.Context, products.CreateInput) (products.Product, error)
	Update(context.Context, int64, products.UpdateInput) (products.Product, error)
	Deactivate(context.Context, int64) error
}

type ProductHandler struct {
	auth  *AuthHandler
	store ProductService
}

func RegisterProductRoutes(
	mux *http.ServeMux,
	authHandler *AuthHandler,
	store ProductService,
) {
	h := &ProductHandler{
		auth:  authHandler,
		store: store,
	}

	mux.Handle(
		"GET /api/v1/products",
		authHandler.Protect(
			"",
			http.HandlerFunc(h.list),
		),
	)

	mux.Handle(
		"GET /api/v1/products/{id}",
		authHandler.Protect(
			"",
			http.HandlerFunc(h.get),
		),
	)

	mux.Handle(
		"POST /api/v1/products",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.create),
		),
	)

	mux.Handle(
		"PUT /api/v1/products/{id}",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.update),
		),
	)

	mux.Handle(
		"POST /api/v1/products/{id}/deactivate",
		authHandler.Protect(
			"admin",
			http.HandlerFunc(h.deactivate),
		),
	)
}

func parseProductID(r *http.Request) (int64, error) {
	raw := r.PathValue("id")

	id, err := strconv.ParseInt(raw, 10, 64)

	if err != nil ||
		id <= 0 ||
		strconv.FormatInt(id, 10) != raw {
		return 0, products.ErrInvalidInput
	}

	return id, nil
}

func decodeProductJSON(
	w http.ResponseWriter,
	r *http.Request,
	destination any,
) error {
	contentType, _, err := mime.ParseMediaType(
		r.Header.Get("Content-Type"),
	)

	if err != nil || contentType != "application/json" {
		return products.ErrInvalidInput
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8192)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return products.ErrInvalidInput
	}

	var extra any

	if err := decoder.Decode(&extra); err != io.EOF {
		return products.ErrInvalidInput
	}

	return nil
}

func (h *ProductHandler) failure(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, products.ErrInvalidInput):
		h.auth.error(
			w, r, 400,
			"INVALID_PRODUCT",
			"Invalid product data",
		)

	case errors.Is(err, products.ErrSKUExists):
		h.auth.error(
			w, r, 409,
			"SKU_EXISTS",
			"Product SKU already exists",
		)

	case errors.Is(err, products.ErrNotFound):
		h.auth.error(
			w, r, 404,
			"PRODUCT_NOT_FOUND",
			"Product not found",
		)

	default:
		h.auth.logger.Error(
			"product_operation_failed",
			"error", err,
		)

		h.auth.error(
			w, r, 503,
			"SERVICE_UNAVAILABLE",
			"Service unavailable",
		)
	}
}

func (h *ProductHandler) list(
	w http.ResponseWriter,
	r *http.Request,
) {
	query := r.URL.Query()

	for key, values := range query {
		switch key {
		case "limit", "offset", "include_inactive":
			if len(values) != 1 {
				h.failure(w, r, products.ErrInvalidInput)
				return
			}
		default:
			h.failure(w, r, products.ErrInvalidInput)
			return
		}
	}

	limit := 20
	offset := 0
	includeInactive := false

	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			h.failure(w, r, products.ErrInvalidInput)
			return
		}
		limit = value
	}

	if raw := query.Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 1000000 {
			h.failure(w, r, products.ErrInvalidInput)
			return
		}
		offset = value
	}

	if raw := query.Get("include_inactive"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			h.failure(w, r, products.ErrInvalidInput)
			return
		}
		includeInactive = value
	}

	identity, _ := IdentityFromContext(r.Context())

	if includeInactive && identity.Role != "admin" {
		h.auth.error(
			w, r, 403,
			"FORBIDDEN",
			"Insufficient permissions",
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		5*time.Second,
	)
	defer cancel()

	page, err := h.store.List(
		ctx, limit, offset, includeInactive,
	)

	if err != nil {
		h.failure(w, r, err)
		return
	}

	writeJSON(w, 200, map[string]any{
		"data": page.Items,
		"pagination": map[string]any{
			"limit":    page.Limit,
			"offset":   page.Offset,
			"has_more": page.HasMore,
		},
	})
}

func (h *ProductHandler) get(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := parseProductID(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		5*time.Second,
	)
	defer cancel()

	p, err := h.store.Get(ctx, id)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	identity, _ := IdentityFromContext(r.Context())

	if !p.IsActive && identity.Role != "admin" {
		h.failure(w, r, products.ErrNotFound)
		return
	}

	writeJSON(w, 200, map[string]any{
		"data": p,
	})
}

func (h *ProductHandler) create(
	w http.ResponseWriter,
	r *http.Request,
) {
	var input products.CreateInput

	if err := decodeProductJSON(
		w, r, &input,
	); err != nil {
		h.failure(w, r, err)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		10*time.Second,
	)
	defer cancel()

	p, err := h.store.Create(ctx, input)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	w.Header().Set(
		"Location",
		"/api/v1/products/"+strconv.FormatInt(p.ID, 10),
	)

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": p,
	})
}

func (h *ProductHandler) update(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := parseProductID(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	// PUT requires all editable fields.
	// Omitting description means clearing it to NULL.
	var payload struct {
		Name              string  `json:"name"`
		Description       *string `json:"description"`
		LowStockThreshold *int64  `json:"low_stock_threshold"`
	}

	if err := decodeProductJSON(
		w, r, &payload,
	); err != nil {
		h.failure(w, r, err)
		return
	}

	if payload.LowStockThreshold == nil {
		h.failure(w, r, products.ErrInvalidInput)
		return
	}

	input := products.UpdateInput{
		Name:              payload.Name,
		Description:       payload.Description,
		LowStockThreshold: *payload.LowStockThreshold,
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		10*time.Second,
	)
	defer cancel()

	p, err := h.store.Update(ctx, id, input)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	writeJSON(w, 200, map[string]any{
		"data": p,
	})
}

func (h *ProductHandler) deactivate(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := parseProductID(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		10*time.Second,
	)
	defer cancel()

	if err := h.store.Deactivate(ctx, id); err != nil {
		h.failure(w, r, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
