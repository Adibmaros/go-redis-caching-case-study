package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/model"
)

type StatsRepository struct {
	db *sql.DB
}

func NewStatsRepository(db *sql.DB) *StatsRepository {
	return &StatsRepository{db: db}
}

func (r *StatsRepository) FindTopProducts(ctx context.Context, limit int) ([]model.TopProduct, error) {
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT p.id, p.name, p.price, SUM(oi.quantity) AS total_sold
		FROM order_items oi
		JOIN products p ON p.id = oi.product_id
		GROUP BY p.id, p.name, p.price
		ORDER BY total_sold DESC
		LIMIT ?;
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top products: %w", err)
	}
	defer rows.Close()

	results := make([]model.TopProduct, 0, limit)
	for rows.Next() {
		var item model.TopProduct
		if err := rows.Scan(&item.ID, &item.Name, &item.Price, &item.TotalSold); err != nil {
			return nil, fmt.Errorf("failed to scan top product: %w", err)
		}
		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during top products iteration: %w", err)
	}

	return results, nil
}

func (r *StatsRepository) FindDashboard(ctx context.Context) (*model.DashboardStats, error) {
	var stats model.DashboardStats

	// 1. Total orders & total revenue
	queryOrders := `
		SELECT COUNT(*) AS total_orders, COALESCE(SUM(total_amount), 0) AS total_revenue
		FROM orders
		WHERE status <> 'cancelled';
	`
	if err := r.db.QueryRowContext(ctx, queryOrders).Scan(&stats.TotalOrders, &stats.TotalRevenue); err != nil {
		return nil, fmt.Errorf("failed to query total orders and revenue: %w", err)
	}

	// 2. Orders today
	queryToday := `
		SELECT COUNT(*)
		FROM orders
		WHERE DATE(created_at) = CURDATE();
	`
	if err := r.db.QueryRowContext(ctx, queryToday).Scan(&stats.OrdersToday); err != nil {
		return nil, fmt.Errorf("failed to query today's orders: %w", err)
	}

	// 3. Total items sold
	queryItems := `
		SELECT COALESCE(SUM(quantity), 0)
		FROM order_items;
	`
	if err := r.db.QueryRowContext(ctx, queryItems).Scan(&stats.TotalItemsSold); err != nil {
		return nil, fmt.Errorf("failed to query total items sold: %w", err)
	}

	return &stats, nil
}
