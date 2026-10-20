package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/cache"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/config"
)

func RateLimiter(cacheMgr *cache.Manager, cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip rate limiting if Redis is disabled or not configured
			if !cacheMgr.IsEnabled() || cfg.RateLimitPerMin <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			clientIP := extractIP(r)
			unixMinute := time.Now().Unix() / 60
			key := fmt.Sprintf("ratelimit:%s:%d", clientIP, unixMinute)

			ctx, cancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
			defer cancel()

			pipe := cacheMgr.Client().Pipeline()
			incrCmd := pipe.Incr(ctx, key)
			_ = pipe.Expire(ctx, key, 65*time.Second) // 65s to safely cover 1-minute window
			_, err := pipe.Exec(ctx)

			if err != nil {
				// Fail open: don't block user requests if Redis fails
				slog.Warn("rate limiter redis error, failing open", "error", err, "client_ip", clientIP)
				next.ServeHTTP(w, r)
				return
			}

			count := incrCmd.Val()
			limit := int64(cfg.RateLimitPerMin)
			remaining := limit - count
			if remaining < 0 {
				remaining = 0
			}

			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(limit, 10))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

			if count > limit {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{
						"message": "rate limit exceeded, please retry after 60 seconds",
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}

	return r.RemoteAddr
}
