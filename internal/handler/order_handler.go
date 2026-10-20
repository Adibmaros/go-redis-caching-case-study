package handler

import (
	"encoding/json"
	"net/http"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/model"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/service"
)

type OrderHandler struct {
	orderService *service.OrderService
}

func NewOrderHandler(orderService *service.OrderService) *OrderHandler {
	return &OrderHandler{orderService: orderService}
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req model.OrderCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.CustomerName == "" {
		Error(w, http.StatusBadRequest, "customer_name is required")
		return
	}
	if len(req.Items) == 0 {
		Error(w, http.StatusBadRequest, "items cannot be empty")
		return
	}

	order, err := h.orderService.CreateOrder(r.Context(), &req)
	if err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	JSON(w, http.StatusCreated, order)
}
