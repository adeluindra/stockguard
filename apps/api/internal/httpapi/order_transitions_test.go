package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adeluindra/stockguard/apps/api/internal/auth"
	"github.com/adeluindra/stockguard/apps/api/internal/orders"
)

// Keep the BE-5A mock compatible with the expanded interface.
func (f *fakeOrderService) Transition(
	_ context.Context,
	_ int64,
	_ int64,
	_ orders.TransitionAction,
) (orders.TransitionResult, error) {
	return orders.TransitionResult{},
		orders.ErrInvalidTransition
}

type fakeTerminalOrders struct {
	calls   int
	actorID int64
	action  orders.TransitionAction
	result  orders.TransitionResult
	err     error
}

func (*fakeTerminalOrders) Create(
	context.Context,
	int64,
	string,
	[]orders.Item,
) (orders.CreateResult, error) {
	return orders.CreateResult{}, nil
}

func (f *fakeTerminalOrders) Transition(
	_ context.Context,
	actorID, orderID int64,
	action orders.TransitionAction,
) (orders.TransitionResult, error) {
	f.calls++
	f.actorID = actorID
	f.action = action

	if f.err != nil {
		return orders.TransitionResult{}, f.err
	}

	result := f.result
	result.Order.ID = orderID

	if action == orders.ActionFulfill {
		result.Order.Status = "fulfilled"
	} else {
		result.Order.Status = "cancelled"
	}

	return result, nil
}

func TestOrderTransitionHTTP(t *testing.T) {
	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	sessions := &productSessionFake{
		role:          "admin",
		authenticated: true,
	}

	authHandler, err := NewAuthHandler(
		&productAccountFake{},
		sessions,
		logger,
		"http://127.0.0.1:8080",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	store := &fakeTerminalOrders{}

	router := NewRouterWithAuthProductsInventoryOrders(
		func(context.Context) error { return nil },
		logger,
		authHandler,
		nil,
		nil,
		store,
	)

	token := strings.Repeat("A", 43)

	send := func(
		path, body string,
		csrf, sameOrigin bool,
	) *httptest.ResponseRecorder {
		req := httptest.NewRequest(
			http.MethodPost,
			path,
			strings.NewReader(body),
		)

		req.AddCookie(&http.Cookie{
			Name:  auth.SessionCookieName,
			Value: token,
		})

		if sameOrigin {
			req.Header.Set(
				"Origin",
				"http://127.0.0.1:8080",
			)
		} else {
			req.Header.Set(
				"Origin",
				"http://invalid.example",
			)
		}

		if csrf {
			req.Header.Set(
				"X-CSRF-Token",
				csrfFromSessionToken(token),
			)
		}

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		return rec
	}

	fulfill := "/api/v1/orders/42/fulfill"
	cancel := "/api/v1/orders/42/cancel"

	sessions.authenticated = false

	if rec := send(
		fulfill, "", true, true,
	); rec.Code != 401 {
		t.Fatalf("unauthenticated: %d", rec.Code)
	}

	sessions.authenticated = true
	sessions.role = "staff"

	if rec := send(
		fulfill, "", true, true,
	); rec.Code != 403 {
		t.Fatalf("staff fulfill: %d", rec.Code)
	}

	if rec := send(
		cancel, "", true, true,
	); rec.Code != 403 {
		t.Fatalf("staff cancel: %d", rec.Code)
	}

	sessions.role = "admin"
	sessions.mustChange = true

	if rec := send(
		fulfill, "", true, true,
	); rec.Code != 403 {
		t.Fatalf(
			"password change required: %d",
			rec.Code,
		)
	}

	sessions.mustChange = false

	if rec := send(
		fulfill, "", false, true,
	); rec.Code != 403 {
		t.Fatalf("missing CSRF: %d", rec.Code)
	}

	if rec := send(
		fulfill, "", true, false,
	); rec.Code != 403 {
		t.Fatalf("wrong Origin: %d", rec.Code)
	}

	if rec := send(
		"/api/v1/orders/nope/fulfill",
		"", true, true,
	); rec.Code != 400 {
		t.Fatalf("invalid ID: %d", rec.Code)
	}

	if rec := send(
		fulfill, `{}`, true, true,
	); rec.Code != 400 {
		t.Fatalf("unexpected body: %d", rec.Code)
	}

	if store.calls != 0 {
		t.Fatalf(
			"store called for invalid requests: %d",
			store.calls,
		)
	}

	rec := send(fulfill, "", true, true)

	if rec.Code != 200 ||
		store.calls != 1 ||
		store.actorID != 1 ||
		store.action != orders.ActionFulfill {
		t.Fatalf(
			"fulfill failed: status=%d calls=%d action=%q",
			rec.Code, store.calls, store.action,
		)
	}

	var payload struct {
		Data struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
		Replayed bool `json:"replayed"`
	}

	if err := json.Unmarshal(
		rec.Body.Bytes(),
		&payload,
	); err != nil {
		t.Fatal(err)
	}

	if payload.Data.ID != 42 ||
		payload.Data.Status != "fulfilled" ||
		payload.Replayed {
		t.Fatalf(
			"wrong fulfillment response: %+v",
			payload,
		)
	}

	store.result.Replayed = true

	rec = send(fulfill, "", true, true)

	if rec.Code != 200 ||
		!strings.Contains(
			rec.Body.String(),
			`"replayed":true`,
		) {
		t.Fatalf(
			"replay failed: %d %s",
			rec.Code,
			rec.Body.String(),
		)
	}

	store.result.Replayed = false

	rec = send(cancel, "", true, true)

	if rec.Code != 200 ||
		store.action != orders.ActionCancel {
		t.Fatalf("cancel failed: %d", rec.Code)
	}

	store.err = orders.ErrOrderNotReserved

	if rec := send(
		cancel, "", true, true,
	); rec.Code != 409 {
		t.Fatalf("terminal conflict: %d", rec.Code)
	}

	store.err = orders.ErrOrderNotFound

	if rec := send(
		fulfill, "", true, true,
	); rec.Code != 404 {
		t.Fatalf("missing order: %d", rec.Code)
	}

	store.err = orders.ErrReservationInconsistent

	if rec := send(
		fulfill, "", true, true,
	); rec.Code != 503 {
		t.Fatalf("broken reservation: %d", rec.Code)
	}
}
