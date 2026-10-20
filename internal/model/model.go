package model

import "time"

type Category struct {
	ID        uint32    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Product struct {
	ID          uint64    `json:"id"`
	CategoryID  uint32    `json:"category_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       float64   `json:"price"`
	Stock       int       `json:"stock"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProductCreateReq struct {
	CategoryID  uint32  `json:"category_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
}

type ProductUpdateReq struct {
	CategoryID  uint32  `json:"category_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
}

type TopProduct struct {
	ID        uint64  `json:"id"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	TotalSold int64   `json:"total_sold"`
}

type DashboardStats struct {
	TotalOrders    int64   `json:"total_orders"`
	TotalRevenue   float64 `json:"total_revenue"`
	OrdersToday    int64   `json:"orders_today"`
	TotalItemsSold int64   `json:"total_items_sold"`
}

type Order struct {
	ID           uint64      `json:"id"`
	CustomerName string      `json:"customer_name"`
	TotalAmount  float64     `json:"total_amount"`
	Status       string      `json:"status"`
	CreatedAt    time.Time   `json:"created_at"`
	Items        []OrderItem `json:"items,omitempty"`
}

type OrderItem struct {
	ID        uint64  `json:"id"`
	OrderID   uint64  `json:"order_id"`
	ProductID uint64  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type OrderCreateReq struct {
	CustomerName string         `json:"customer_name"`
	Items        []OrderItemReq `json:"items"`
}

type OrderItemReq struct {
	ProductID uint64 `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type ProductListResponse struct {
	Items      []Product `json:"items"`
	Page       int       `json:"page"`
	Limit      int       `json:"limit"`
	Total      int64     `json:"total"`
	TotalPages int       `json:"total_pages"`
}
