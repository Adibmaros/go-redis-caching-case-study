package service

import (
	"context"
	"fmt"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/cache"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/config"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/model"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/repository"
)

type StatsService struct {
	repo  *repository.StatsRepository
	cache *cache.Manager
	cfg   *config.Config
}

func NewStatsService(repo *repository.StatsRepository, cache *cache.Manager, cfg *config.Config) *StatsService {
	return &StatsService{
		repo:  repo,
		cache: cache,
		cfg:   cfg,
	}
}

func (s *StatsService) GetTopProducts(ctx context.Context, limit int) ([]model.TopProduct, cache.CacheStatus, error) {
	if !s.cache.IsEnabled() {
		data, err := s.repo.FindTopProducts(ctx, limit)
		return data, cache.CacheBypass, err
	}

	cacheKey := fmt.Sprintf("stats:top_products:l:%d", limit)

	// Check cache
	cached, found, _ := cache.GetJSON[[]model.TopProduct](ctx, s.cache, cacheKey)
	if found && cached != nil {
		return cached, cache.CacheHit, nil
	}

	// Singleflight protection against heavy database aggregation load
	val, err := s.cache.SingleFlight(cacheKey, func() (any, error) {
		if c, found, _ := cache.GetJSON[[]model.TopProduct](ctx, s.cache, cacheKey); found && c != nil {
			return c, nil
		}

		data, err := s.repo.FindTopProducts(ctx, limit)
		if err != nil {
			return nil, err
		}

		_ = cache.SetJSON(ctx, s.cache, cacheKey, data, s.cfg.TTLTopProducts)
		return data, nil
	})

	if err != nil {
		return nil, cache.CacheMiss, err
	}

	return val.([]model.TopProduct), cache.CacheMiss, nil
}

func (s *StatsService) GetDashboard(ctx context.Context) (*model.DashboardStats, cache.CacheStatus, error) {
	if !s.cache.IsEnabled() {
		data, err := s.repo.FindDashboard(ctx)
		return data, cache.CacheBypass, err
	}

	cacheKey := "stats:dashboard"

	// Check cache
	cached, found, _ := cache.GetJSON[*model.DashboardStats](ctx, s.cache, cacheKey)
	if found && cached != nil {
		return cached, cache.CacheHit, nil
	}

	// Singleflight
	val, err := s.cache.SingleFlight(cacheKey, func() (any, error) {
		if c, found, _ := cache.GetJSON[*model.DashboardStats](ctx, s.cache, cacheKey); found && c != nil {
			return c, nil
		}

		data, err := s.repo.FindDashboard(ctx)
		if err != nil {
			return nil, err
		}

		_ = cache.SetJSON(ctx, s.cache, cacheKey, data, s.cfg.TTLDashboard)
		return data, nil
	})

	if err != nil {
		return nil, cache.CacheMiss, err
	}

	return val.(*model.DashboardStats), cache.CacheMiss, nil
}
