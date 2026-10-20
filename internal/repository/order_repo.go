package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/model"
)

type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Create(ctx context.Context, req *model.OrderCreateReq) (*model.Order, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("order must contain at least one item")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin order transaction: %w", err)
	}
	defer tx.Rollback()

	var totalAmount float64
	orderItems := make([]model.OrderItem, 0, len(req.Items))

	for _, item := range req.Items {
		if item.Quantity <= 0 {
			return nil, fmt.Errorf("item quantity must be greater than 0 for product %d", item.ProductID)
		}

		var price float64
		var stock int
		err := tx.QueryRowContext(ctx, "SELECT price, stock FROM products WHERE id = ? FOR UPDATE", item.ProductID).Scan(&price, &stock)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("product %d not found", item.ProductID)
			}
			return nil, fmt.Errorf("failed to query product %d: %w", item.ProductID, err)
		}

		if stock < item.Quantity {
			return nil, fmt.Errorf("insufficient stock for product %d (available: %d, requested: %d)", item.ProductID, stock, item.Quantity)
		}

		// Deduct stock
		_, err = tx.ExecContext(ctx, "UPDATE products SET stock = stock - ? WHERE id = ?", item.Quantity, item.ProductID)
		if err != nil {
			return nil, fmt.Errorf("failed to update stock for product %d: %w", item.ProductID, err)
		}

		subtotal := price * float64(item.Quantity)
		totalAmount += subtotal

		orderItems = append(orderItems, model.OrderItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			UnitPrice: price,
		})
	}

	// Insert order
	res, err := tx.ExecContext(ctx,
		"INSERT INTO orders (customer_name, total_amount, status) VALUES (?, ?, 'paid')",
		req.CustomerName,
		totalAmount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert order: %w", err)
	}

	orderID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get order id: %w", err)
	}

	// Insert order items
	itemStmt, err := tx.PrepareContext(ctx, "INSERT INTO order_items (order_id, product_id, quantity, unit_price) VALUES (?, ?, ?, ?)")
	if err != nil {
		return nil, fmt.Errorf("failed to prepare order items statement: %w", err)
	}
	defer itemStmt.Close()

	for i := range orderItems {
		orderItems[i].OrderID = uint64(orderID)
		resItem, err := itemStmt.ExecContext(ctx, orderID, orderItems[i].ProductID, orderItems[i].Quantity, orderItems[i].UnitPrice)
		if err != nil {
			return nil, fmt.Errorf("failed to insert order item: %w", err)
		}
		itemID, err := resItem.LastInsertId()
		if err == nil {
			orderItems[i].ID = uint64(itemID)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit order transaction: %w", err)
	}

	return &model.Order{
		ID:           uint64(orderID),
		CustomerName: req.CustomerName,
		TotalAmount:  totalAmount,
		Status:       "paid",
		CreatedAt:    time.Now(),
		Items:        orderItems,
	}, nil
}
