package handler

import (
	"net/http"
	"time"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/service"
)

type StatsHandler struct {
	statsService *service.StatsService
}

func NewStatsHandler(statsService *service.StatsService) *StatsHandler {
	return &StatsHandler{statsService: statsService}
}

func (h *StatsHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	dashboard, cacheStatus, err := h.statsService.GetDashboard(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSONWithCache(w, http.StatusOK, dashboard, cacheStatus, start)
}
