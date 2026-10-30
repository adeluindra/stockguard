package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type ReadinessCheck func(context.Context) error

type requestIDKey struct{}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func NewRouter(
	checkReady ReadinessCheck,
	logger *slog.Logger,
) http.Handler {
	return newRouter(checkReady, logger, nil)
}

func NewRouterWithAuth(
	checkReady ReadinessCheck,
	logger *slog.Logger,
	handlers *AuthHandler,
) http.Handler {
	return newRouter(checkReady, logger, handlers)
}

func newRouter(
	checkReady ReadinessCheck,
	logger *slog.Logger,
	handlers *AuthHandler,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(
		"GET /healthz",
		func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 200, map[string]any{
				"data": map[string]string{
					"status": "ok",
				},
			})
		},
	)

	mux.HandleFunc(
		"GET /readyz",
		func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(
				r.Context(),
				2*time.Second,
			)
			defer cancel()

			if err := checkReady(ctx); err != nil {
				logger.Warn(
					"readiness_check_failed",
					"error", err,
				)

				requestID, _ := r.Context().
					Value(requestIDKey{}).(string)

				writeJSON(w, 503, map[string]any{
					"error": map[string]any{
						"code":       "SERVICE_UNAVAILABLE",
						"message":    "Service is not ready",
						"details":    []any{},
						"request_id": requestID,
					},
				})
				return
			}

			writeJSON(w, 200, map[string]any{
				"data": map[string]string{
					"status":   "ok",
					"database": "ok",
				},
			})
		},
	)

	if handlers != nil {
		// Existing BE-2 authentication endpoints.
		handlers.Register(mux)

		// BE-2F: Admin-only Staff management endpoints.
		handlers.RegisterStaffRoutes(mux)
	}

	return requestLogging(logger, mux)
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	payload any,
) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(
			w,
			"Internal Server Error",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	_, _ = w.Write(append(body, '\n'))
}

func requestLogging(
	logger *slog.Logger,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		started := time.Now()

		var randomBytes [12]byte

		if _, err := rand.Read(randomBytes[:]); err != nil {
			logger.Error(
				"request_id_rng_failed",
				"error", err,
			)

			writeJSON(w, 500, map[string]any{
				"error": "Internal server error",
			})
			return
		}

		requestID := hex.EncodeToString(randomBytes[:])

		w.Header().Set("X-Request-ID", requestID)

		ctx := context.WithValue(
			r.Context(),
			requestIDKey{},
			requestID,
		)

		recorder := &statusWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(
			recorder,
			r.WithContext(ctx),
		)

		logger.Info(
			"http_request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms",
			time.Since(started).Milliseconds(),
		)
	})
}
