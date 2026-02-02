package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/safar/go-sql-store/internal/database"
	"github.com/safar/go-sql-store/internal/models"
	"github.com/shopspring/decimal"
)

func CreateProduct(ctx context.Context, db *sql.DB, sku, name, description string, price decimal.Decimal, stock int) (*models.Product, error) {
	product := &models.Product{}

	query := `
		INSERT INTO products (sku, name, description, price, stock_quantity, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW(), 1)
		RETURNING id, sku, name, description, price, stock_quantity, created_at, updated_at, version`

	err := db.QueryRowContext(ctx, query, sku, name, description, price, stock).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Description,
		&product.Price,
		&product.StockQuantity,
		&product.CreatedAt,
		&product.UpdatedAt,
		&product.Version,
	)
	if err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}

	return product, nil
}

func GetProduct(ctx context.Context, db *sql.DB, id int64) (*models.Product, error) {
	product := &models.Product{}

	query := `
		SELECT id, sku, name, description, price, stock_quantity, created_at, updated_at, version
		FROM products
		WHERE id = $1`

	err := db.QueryRowContext(ctx, query, id).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Description,
		&product.Price,
		&product.StockQuantity,
		&product.CreatedAt,
		&product.UpdatedAt,
		&product.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, database.ErrProductNotFound
		}
		return nil, fmt.Errorf("get product: %w", err)
	}

	return product, nil
}

func ReserveStock(ctx context.Context, tx *sql.Tx, productID int64, quantity int) (*models.Product, error) {
	product := &models.Product{}

	query := `
		SELECT id, sku, name, description, price, stock_quantity, created_at, updated_at, version
		FROM products
		WHERE id = $1
		FOR UPDATE`

	err := tx.QueryRowContext(ctx, query, productID).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Description,
		&product.Price,
		&product.StockQuantity,
		&product.CreatedAt,
		&product.UpdatedAt,
		&product.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, database.ErrProductNotFound
		}
		return nil, fmt.Errorf("lock product: %w", err)
	}

	if product.StockQuantity < quantity {
		return nil, database.ErrInsufficientStock
	}

	return product, nil
}

func ReserveStockNoWait(ctx context.Context, tx *sql.Tx, productID int64, quantity int) (*models.Product, error) {
	product := &models.Product{}

	query := `
		SELECT id, sku, name, description, price, stock_quantity, created_at, updated_at, version
		FROM products
		WHERE id = $1
		FOR UPDATE NOWAIT`

	err := tx.QueryRowContext(ctx, query, productID).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Description,
		&product.Price,
		&product.StockQuantity,
		&product.CreatedAt,
		&product.UpdatedAt,
		&product.Version,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "55P03" {
			return nil, database.ErrLockTimeout
		}
		if err == sql.ErrNoRows {
			return nil, database.ErrProductNotFound
		}
		return nil, fmt.Errorf("lock product (nowait): %w", err)
	}

	if product.StockQuantity < quantity {
		return nil, database.ErrInsufficientStock
	}

	return product, nil
}

func UpdateStockOptimistic(ctx context.Context, db *sql.DB, productID int64, newStock int, version int) error {
	result, err := db.ExecContext(ctx,
		`UPDATE products
		 SET stock_quantity = $1, version = version + 1, updated_at = NOW()
		 WHERE id = $2 AND version = $3`,
		newStock, productID, version)
	if err != nil {
		return fmt.Errorf("update stock: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return database.ErrOptimisticLockFailed
	}

	return nil
}

func DecrementStock(ctx context.Context, tx *sql.Tx, productID int64, quantity int) error {
	result, err := tx.ExecContext(ctx,
		`UPDATE products
		 SET stock_quantity = stock_quantity - $1,
		     updated_at = NOW()
		 WHERE id = $2
		   AND stock_quantity >= $1`,
		quantity, productID)
	if err != nil {
		return fmt.Errorf("decrement stock: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return database.ErrInsufficientStock
	}

	return nil
}

func ListProducts(ctx context.Context, db *sql.DB, page, pageSize int) (*OffsetPage, error) {
	var total int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products`).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("count products: %w", err)
	}

	offset := (page - 1) * pageSize
	query := `
		SELECT id, sku, name, description, price, stock_quantity, created_at, updated_at, version
		FROM products
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := db.QueryContext(ctx, query, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var product models.Product
		err := rows.Scan(
			&product.ID,
			&product.SKU,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.StockQuantity,
			&product.CreatedAt,
			&product.UpdatedAt,
			&product.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &OffsetPage{
		Items:      products,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}
