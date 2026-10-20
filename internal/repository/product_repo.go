package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/Adibmaros/go-redis-caching-case-study/internal/model"
)

var ErrNotFound = errors.New("record not found")

type ProductRepository struct {
	db *sql.DB
}

func NewProductRepository(db *sql.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

func (r *ProductRepository) FindByID(ctx context.Context, id uint64) (*model.Product, error) {
	query := `
		SELECT id, category_id, name, description, price, stock, created_at, updated_at
		FROM products
		WHERE id = ?
	`

	var p model.Product
	var desc sql.NullString
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID,
		&p.CategoryID,
		&p.Name,
		&desc,
		&p.Price,
		&p.Stock,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to query product by id: %w", err)
	}

	if desc.Valid {
		p.Description = desc.String
	}

	return &p, nil
}

func (r *ProductRepository) FindList(ctx context.Context, categoryID uint32, page, limit int) (*model.ProductListResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var countQuery string
	var listQuery string
	var countArgs []any
	var listArgs []any

	if categoryID > 0 {
		countQuery = "SELECT COUNT(*) FROM products WHERE category_id = ?"
		countArgs = append(countArgs, categoryID)

		listQuery = `
			SELECT id, category_id, name, description, price, stock, created_at, updated_at
			FROM products
			WHERE category_id = ?
			ORDER BY id ASC
			LIMIT ? OFFSET ?
		`
		listArgs = append(listArgs, categoryID, limit, offset)
	} else {
		countQuery = "SELECT COUNT(*) FROM products"

		listQuery = `
			SELECT id, category_id, name, description, price, stock, created_at, updated_at
			FROM products
			ORDER BY id ASC
			LIMIT ? OFFSET ?
		`
		listArgs = append(listArgs, limit, offset)
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count products: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query products list: %w", err)
	}
	defer rows.Close()

	items := make([]model.Product, 0, limit)
	for rows.Next() {
		var p model.Product
		var desc sql.NullString
		if err := rows.Scan(
			&p.ID,
			&p.CategoryID,
			&p.Name,
			&desc,
			&p.Price,
			&p.Stock,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan product row: %w", err)
		}
		if desc.Valid {
			p.Description = desc.String
		}
		items = append(items, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during products iteration: %w", err)
	}

	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(limit)))
	}

	return &model.ProductListResponse{
		Items:      items,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (r *ProductRepository) Create(ctx context.Context, req *model.ProductCreateReq) (*model.Product, error) {
	query := `
		INSERT INTO products (category_id, name, description, price, stock)
		VALUES (?, ?, ?, ?, ?)
	`

	res, err := r.db.ExecContext(ctx, query, req.CategoryID, req.Name, req.Description, req.Price, req.Stock)
	if err != nil {
		return nil, fmt.Errorf("failed to insert product: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve last insert id: %w", err)
	}

	return r.FindByID(ctx, uint64(id))
}

func (r *ProductRepository) Update(ctx context.Context, id uint64, req *model.ProductUpdateReq) (*model.Product, error) {
	query := `
		UPDATE products
		SET category_id = ?, name = ?, description = ?, price = ?, stock = ?
		WHERE id = ?
	`

	res, err := r.db.ExecContext(ctx, query, req.CategoryID, req.Name, req.Description, req.Price, req.Stock, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update product: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		// Check if product exists
		if _, err := r.FindByID(ctx, id); err != nil {
			return nil, err
		}
	}

	return r.FindByID(ctx, id)
}

func (r *ProductRepository) Delete(ctx context.Context, id uint64) error {
	// First check existence
	if _, err := r.FindByID(ctx, id); err != nil {
		return err
	}

	query := "DELETE FROM products WHERE id = ?"
	if _, err := r.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("failed to delete product: %w", err)
	}

	return nil
}
