package service

import (
	"context"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/model"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/repository"
)

type OrderService struct {
	repo *repository.OrderRepository
}

func NewOrderService(repo *repository.OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) CreateOrder(ctx context.Context, req *model.OrderCreateReq) (*model.Order, error) {
	// Catatan desain sengaja: Cache agregasi (top products dan dashboard) TIDAK di-invalidasi
	// di sini. Kita mengandalkan TTL (eventual consistency) sehingga endpoint statistik
	// tidak dibebani query agregasi ulang setiap kali order baru masuk.
	return s.repo.Create(ctx, req)
}
