package web

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (h *Handler) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'")
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
		h.log.Info("HTTP 访问", "method", r.Method, "path", sanitizePath(r.URL.Path), "duration_ms", time.Since(started).Milliseconds())
	})
}

func sanitizePath(path string) string {
	path = strings.ReplaceAll(path, "\n", "")
	path = strings.ReplaceAll(path, "\r", "")
	if len(path) > 180 {
		return path[:180]
	}
	return path
}
