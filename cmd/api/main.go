package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/cache"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/config"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/database"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/handler"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/middleware"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/repository"
	"github.com/Adibmaros/go-redis-caching-case-study/internal/service"
)

func main() {
	// 1. Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// 2. Load configuration
	cfg := config.Load()
	slog.Info("Configuration loaded",
		"port", cfg.AppPort,
		"host", cfg.AppHost,
		"env", cfg.AppEnv,
		"cache_enabled", cfg.CacheEnabled,
		"redis_addr", cfg.RedisAddr,
	)

	// 3. Initialize MySQL
	db, err := database.NewMySQL(cfg)
	if err != nil {
		slog.Error("MySQL connection failed. Ensure MySQL is running on your host", "error", err)
		// We still allow startup so user can run API even if DB needs config update, or log fatal
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("MySQL connection pool established successfully")

	// 4. Initialize Redis (toleran jika belum aktif)
	var cacheMgr *cache.Manager
	if cfg.CacheEnabled {
		rdb, err := cache.NewRedis(cfg)
		if err != nil {
			slog.Warn("Redis connection failed. Running in fallback mode (direct to MySQL)", "error", err)
			cacheMgr = cache.NewManager(nil, false)
		} else {
			slog.Info("Redis connected successfully", "addr", cfg.RedisAddr)
			cacheMgr = cache.NewManager(rdb, true)
		}
	} else {
		slog.Info("Cache is explicitly disabled via CACHE_ENABLED=false")
		cacheMgr = cache.NewManager(nil, false)
	}

	// 5. Initialize Repositories
	productRepo := repository.NewProductRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	statsRepo := repository.NewStatsRepository(db)

	// 6. Initialize Services
	productService := service.NewProductService(productRepo, cacheMgr, cfg)
	orderService := service.NewOrderService(orderRepo)
	statsService := service.NewStatsService(statsRepo, cacheMgr, cfg)

	// 7. Initialize Handlers
	productHandler := handler.NewProductHandler(productService, statsService)
	orderHandler := handler.NewOrderHandler(orderService)
	statsHandler := handler.NewStatsHandler(statsService)
	debugHandler := handler.NewDebugHandler(db, cacheMgr, cfg)

	// 8. Router Setup
	r := chi.NewRouter()

	// Middlewares
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.Logger)
	r.Use(middleware.RateLimiter(cacheMgr, cfg))

	// Routes
	r.Get("/health", debugHandler.HealthCheck)
	r.Delete("/debug/cache", debugHandler.FlushCache)

	// Important: /products/top must be defined BEFORE /products/{id} to prevent routing conflict
	r.Get("/products/top", productHandler.GetTopProducts)
	r.Get("/products", productHandler.GetProductList)
	r.Get("/products/{id}", productHandler.GetProduct)
	r.Post("/products", productHandler.CreateProduct)
	r.Put("/products/{id}", productHandler.UpdateProduct)
	r.Delete("/products/{id}", productHandler.DeleteProduct)

	r.Post("/orders", orderHandler.CreateOrder)

	r.Get("/stats/dashboard", statsHandler.GetDashboard)

	// 9. HTTP Server with Graceful Shutdown
	addr := fmt.Sprintf("%s:%s", cfg.AppHost, cfg.AppPort)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("Server listening", "addr", addr, "url", fmt.Sprintf("http://%s:%s", cfg.AppHost, cfg.AppPort))
		serverErrors <- srv.ListenAndServe()
	}()

	// Listen for OS signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server error", "error", err)
		}
	case sig := <-shutdown:
		slog.Info("Shutdown signal received", "signal", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Graceful shutdown failed, forcing server close", "error", err)
			_ = srv.Close()
		}
		slog.Info("Server stopped cleanly")
	}
}
