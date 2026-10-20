package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/model"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/repository"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/service"
)

type ProductHandler struct {
	productService *service.ProductService
	statsService   *service.StatsService
}

func NewProductHandler(productService *service.ProductService, statsService *service.StatsService) *ProductHandler {
	return &ProductHandler{
		productService: productService,
		statsService:   statsService,
	}
}

func (h *ProductHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid product id")
		return
	}

	product, cacheStatus, err := h.productService.GetProduct(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			Error(w, http.StatusNotFound, "product not found")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSONWithCache(w, http.StatusOK, product, cacheStatus, start)
}

func (h *ProductHandler) GetProductList(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var categoryID uint32
	if catStr := r.URL.Query().Get("category_id"); catStr != "" {
		if cat, err := strconv.ParseUint(catStr, 10, 32); err == nil {
			categoryID = uint32(cat)
		}
	}

	page := 1
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 20
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	list, cacheStatus, err := h.productService.GetProductList(r.Context(), categoryID, page, limit)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSONWithCache(w, http.StatusOK, list, cacheStatus, start)
}

func (h *ProductHandler) GetTopProducts(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	limit := 10
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	top, cacheStatus, err := h.statsService.GetTopProducts(r.Context(), limit)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSONWithCache(w, http.StatusOK, top, cacheStatus, start)
}

func (h *ProductHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req model.ProductCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Name == "" {
		Error(w, http.StatusBadRequest, "product name is required")
		return
	}
	if req.Price < 0 {
		Error(w, http.StatusBadRequest, "product price cannot be negative")
		return
	}
	if req.CategoryID == 0 {
		Error(w, http.StatusBadRequest, "valid category_id is required")
		return
	}

	product, err := h.productService.CreateProduct(r.Context(), &req)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusCreated, product)
}

func (h *ProductHandler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid product id")
		return
	}

	var req model.ProductUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Name == "" {
		Error(w, http.StatusBadRequest, "product name is required")
		return
	}
	if req.Price < 0 {
		Error(w, http.StatusBadRequest, "product price cannot be negative")
		return
	}
	if req.CategoryID == 0 {
		Error(w, http.StatusBadRequest, "valid category_id is required")
		return
	}

	product, err := h.productService.UpdateProduct(r.Context(), id, &req)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			Error(w, http.StatusNotFound, "product not found")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, product)
}

func (h *ProductHandler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid product id")
		return
	}

	if err := h.productService.DeleteProduct(r.Context(), id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			Error(w, http.StatusNotFound, "product not found")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]string{"message": "product deleted successfully"})
}
