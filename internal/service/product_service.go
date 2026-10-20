package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/cache"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/config"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/model"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/repository"
)

const (
	productListVersionKey = "products:list:version"
)

type ProductService struct {
	repo  *repository.ProductRepository
	cache *cache.Manager
	cfg   *config.Config
}

func NewProductService(repo *repository.ProductRepository, cache *cache.Manager, cfg *config.Config) *ProductService {
	return &ProductService{
		repo:  repo,
		cache: cache,
		cfg:   cfg,
	}
}

func (s *ProductService) GetProduct(ctx context.Context, id uint64) (*model.Product, cache.CacheStatus, error) {
	if !s.cache.IsEnabled() {
		p, err := s.repo.FindByID(ctx, id)
		return p, cache.CacheBypass, err
	}

	cacheKey := fmt.Sprintf("product:%d", id)

	// Check cache
	cached, found, _ := cache.GetJSON[*model.Product](ctx, s.cache, cacheKey)
	if found && cached != nil {
		return cached, cache.CacheHit, nil
	}

	// SingleFlight protection against cache stampede
	val, err := s.cache.SingleFlight(cacheKey, func() (any, error) {
		// Double check cache in singleflight callback
		if c, found, _ := cache.GetJSON[*model.Product](ctx, s.cache, cacheKey); found && c != nil {
			return c, nil
		}

		p, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}

		_ = cache.SetJSON(ctx, s.cache, cacheKey, p, s.cfg.TTLProductDetail)
		return p, nil
	})

	if err != nil {
		return nil, cache.CacheMiss, err
	}

	return val.(*model.Product), cache.CacheMiss, nil
}

func (s *ProductService) GetProductList(ctx context.Context, categoryID uint32, page, limit int) (*model.ProductListResponse, cache.CacheStatus, error) {
	if !s.cache.IsEnabled() {
		list, err := s.repo.FindList(ctx, categoryID, page, limit)
		return list, cache.CacheBypass, err
	}

	ver := s.cache.GetVersion(ctx, productListVersionKey)
	catStr := "all"
	if categoryID > 0 {
		catStr = strconv.FormatUint(uint64(categoryID), 10)
	}

	cacheKey := fmt.Sprintf("products:list:v%d:cat:%s:p:%d:l:%d", ver, catStr, page, limit)

	// Check cache
	cached, found, _ := cache.GetJSON[*model.ProductListResponse](ctx, s.cache, cacheKey)
	if found && cached != nil {
		return cached, cache.CacheHit, nil
	}

	// Singleflight
	val, err := s.cache.SingleFlight(cacheKey, func() (any, error) {
		if c, found, _ := cache.GetJSON[*model.ProductListResponse](ctx, s.cache, cacheKey); found && c != nil {
			return c, nil
		}

		list, err := s.repo.FindList(ctx, categoryID, page, limit)
		if err != nil {
			return nil, err
		}

		_ = cache.SetJSON(ctx, s.cache, cacheKey, list, s.cfg.TTLProductList)
		return list, nil
	})

	if err != nil {
		return nil, cache.CacheMiss, err
	}

	return val.(*model.ProductListResponse), cache.CacheMiss, nil
}

func (s *ProductService) CreateProduct(ctx context.Context, req *model.ProductCreateReq) (*model.Product, error) {
	product, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, err
	}

	// Invalidate list cache by bumping version counter
	_, _ = s.cache.IncrVersion(ctx, productListVersionKey)

	return product, nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, id uint64, req *model.ProductUpdateReq) (*model.Product, error) {
	product, err := s.repo.Update(ctx, id, req)
	if err != nil {
		return nil, err
	}

	// Invalidate specific product cache
	_ = s.cache.Delete(ctx, fmt.Sprintf("product:%d", id))

	// Invalidate list cache by bumping version counter
	_, _ = s.cache.IncrVersion(ctx, productListVersionKey)

	return product, nil
}

func (s *ProductService) DeleteProduct(ctx context.Context, id uint64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// Invalidate specific product cache
	_ = s.cache.Delete(ctx, fmt.Sprintf("product:%d", id))

	// Invalidate list cache by bumping version counter
	_, _ = s.cache.IncrVersion(ctx, productListVersionKey)

	return nil
}
