package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/cache"
)

type SuccessResponse struct {
	Data any `json:"data"`
}

type ErrorDetail struct {
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(SuccessResponse{Data: data})
}

func JSONWithCache(w http.ResponseWriter, status int, data any, cacheStatus cache.CacheStatus, startTime time.Time) {
	duration := time.Since(startTime)
	w.Header().Set("X-Cache", string(cacheStatus))
	w.Header().Set("X-Response-Time", fmt.Sprintf("%.2fms", float64(duration.Microseconds())/1000.0))
	JSON(w, status, data)
}

func Error(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{Message: message},
	})
}
