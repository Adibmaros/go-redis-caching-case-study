package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/cache"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/config"
)

type DebugHandler struct {
	db    *sql.DB
	cache *cache.Manager
	cfg   *config.Config
}

func NewDebugHandler(db *sql.DB, cache *cache.Manager, cfg *config.Config) *DebugHandler {
	return &DebugHandler{
		db:    db,
		cache: cache,
		cfg:   cfg,
	}
}

func (h *DebugHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	mysqlStatus := "UP"
	if err := h.db.PingContext(ctx); err != nil {
		mysqlStatus = "DOWN: " + err.Error()
	}

	redisStatus := "DISABLED"
	if h.cache.IsEnabled() {
		if err := h.cache.Client().Ping(ctx).Err(); err != nil {
			redisStatus = "DOWN: " + err.Error()
		} else {
			redisStatus = "UP"
		}
	}

	statusCode := http.StatusOK
	if mysqlStatus != "UP" {
		statusCode = http.StatusServiceUnavailable
	}

	JSON(w, statusCode, map[string]any{
		"status": "ok",
		"mysql":  mysqlStatus,
		"redis":  redisStatus,
		"cache":  h.cfg.CacheEnabled,
	})
}

func (h *DebugHandler) FlushCache(w http.ResponseWriter, r *http.Request) {
	if h.cfg.AppEnv != "development" {
		Error(w, http.StatusForbidden, "debug cache flush is only allowed in development environment")
		return
	}

	if !h.cache.IsEnabled() {
		Error(w, http.StatusBadRequest, "cache is currently disabled")
		return
	}

	if err := h.cache.FlushDB(r.Context()); err != nil {
		Error(w, http.StatusInternalServerError, "failed to flush cache: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]string{
		"message": "cache flushed successfully",
	})
}
