package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/safar/go-sql-store/internal/database"
	"github.com/safar/go-sql-store/internal/models"
	"github.com/shopspring/decimal"
)

type CreateOrderRequest struct {
	UserID int64
	Items  []OrderItemRequest
}

type OrderItemRequest struct {
	ProductID int64
	Quantity  int
}

func generateOrderNumber() string {
	return fmt.Sprintf("ORD-%d", time.Now().UnixNano())
}

func CreateOrder(ctx context.Context, db *sql.DB, req CreateOrderRequest) (*models.Order, error) {
	var order *models.Order

	err := database.WithRetry(ctx, db, database.TxOptions{
		IsolationLevel: sql.LevelSerializable,
		MaxRetries:     3,
	}, func(tx *sql.Tx) error {
		var exists bool
		err := tx.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)",
			req.UserID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check user exists: %w", err)
		}
		if !exists {
			return database.ErrUserNotFound
		}

		var totalAmount decimal.Decimal
		productPrices := make(map[int64]decimal.Decimal)

		for _, item := range req.Items {
			var productID int64
			var price decimal.Decimal
			var stockQuantity int

			err := tx.QueryRowContext(ctx,
				`SELECT id, price, stock_quantity
				 FROM products
				 WHERE id = $1
				 FOR UPDATE NOWAIT`,
				item.ProductID).Scan(&productID, &price, &stockQuantity)
			if err != nil {
				if err == sql.ErrNoRows {
					return database.ErrProductNotFound
				}
				return fmt.Errorf("lock product %d: %w", item.ProductID, err)
			}

			if stockQuantity < item.Quantity {
				return database.ErrInsufficientStock
			}

			productPrices[item.ProductID] = price
			totalAmount = totalAmount.Add(price.Mul(decimal.NewFromInt(int64(item.Quantity))))
		}

		orderNumber := generateOrderNumber()
		var orderID int64
		err = tx.QueryRowContext(ctx,
			`INSERT INTO orders (user_id, order_number, status, total_amount, created_at, updated_at, version)
			 VALUES ($1, $2, $3, $4, NOW(), NOW(), 1)
			 RETURNING id`,
			req.UserID, orderNumber, models.OrderStatusPending, totalAmount).Scan(&orderID)
		if err != nil {
			return fmt.Errorf("create order: %w", err)
		}

		for _, item := range req.Items {
			unitPrice := productPrices[item.ProductID]
			subtotal := unitPrice.Mul(decimal.NewFromInt(int64(item.Quantity)))

			_, err = tx.ExecContext(ctx,
				`INSERT INTO order_items (order_id, product_id, quantity, unit_price, subtotal, created_at)
				 VALUES ($1, $2, $3, $4, $5, NOW())`,
				orderID, item.ProductID, item.Quantity, unitPrice, subtotal)
			if err != nil {
				return fmt.Errorf("create order item: %w", err)
			}
		}

		for _, item := range req.Items {
			result, err := tx.ExecContext(ctx,
				`UPDATE products
				 SET stock_quantity = stock_quantity - $1,
				     updated_at = NOW()
				 WHERE id = $2
				   AND stock_quantity >= $1`,
				item.Quantity, item.ProductID)
			if err != nil {
				return fmt.Errorf("update stock: %w", err)
			}

			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("get rows affected: %w", err)
			}

			if rowsAffected == 0 {
				return database.ErrInsufficientStock
			}
		}

		order = &models.Order{ID: orderID}
		err = tx.QueryRowContext(ctx,
			`SELECT order_number, user_id, status, total_amount, created_at, updated_at, version
			 FROM orders WHERE id = $1`,
			orderID).Scan(
			&order.OrderNumber,
			&order.UserID,
			&order.Status,
			&order.TotalAmount,
			&order.CreatedAt,
			&order.UpdatedAt,
			&order.Version,
		)
		if err != nil {
			return fmt.Errorf("fetch created order: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return order, nil
}

func GetOrder(ctx context.Context, db *sql.DB, id int64) (*models.Order, error) {
	order := &models.Order{}

	query := `
		SELECT id, user_id, order_number, status, total_amount, created_at, updated_at, version
		FROM orders
		WHERE id = $1`

	err := db.QueryRowContext(ctx, query, id).Scan(
		&order.ID,
		&order.UserID,
		&order.OrderNumber,
		&order.Status,
		&order.TotalAmount,
		&order.CreatedAt,
		&order.UpdatedAt,
		&order.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, database.ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order: %w", err)
	}

	itemsQuery := `
		SELECT id, order_id, product_id, quantity, unit_price, subtotal, created_at
		FROM order_items
		WHERE order_id = $1`

	rows, err := db.QueryContext(ctx, itemsQuery, id)
	if err != nil {
		return nil, fmt.Errorf("get order items: %w", err)
	}
	defer rows.Close()

	var items []models.OrderItem
	for rows.Next() {
		var item models.OrderItem
		err := rows.Scan(
			&item.ID,
			&item.OrderID,
			&item.ProductID,
			&item.Quantity,
			&item.UnitPrice,
			&item.Subtotal,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan order item: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	order.Items = items

	return order, nil
}

func ListOrdersCursor(ctx context.Context, db *sql.DB, userID int64, cursor string, limit int) (*CursorPage, error) {
	cursorData, err := DecodeCursor(cursor)
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}

	query := `
		SELECT id, order_number, status, total_amount, created_at, updated_at, version
		FROM orders
		WHERE user_id = $1
		  AND (created_at, id) < ($2, $3)
		ORDER BY created_at DESC, id DESC
		LIMIT $4`

	rows, err := db.QueryContext(ctx, query, userID, cursorData.CreatedAt, cursorData.ID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		err := rows.Scan(
			&order.ID,
			&order.OrderNumber,
			&order.Status,
			&order.TotalAmount,
			&order.CreatedAt,
			&order.UpdatedAt,
			&order.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	hasMore := len(orders) > limit
	if hasMore {
		orders = orders[:limit]
	}

	var nextCursor string
	if hasMore && len(orders) > 0 {
		lastOrder := orders[len(orders)-1]
		nextCursor = EncodeCursor(OrderCursor{
			CreatedAt: lastOrder.CreatedAt,
			ID:        lastOrder.ID,
		})
	}

	return &CursorPage{
		Items:      orders,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func GetNextPendingOrder(ctx context.Context, tx *sql.Tx) (*models.Order, error) {
	order := &models.Order{}

	query := `
		SELECT id, user_id, order_number, status, total_amount, created_at, updated_at, version
		FROM orders
		WHERE status = $1
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1`

	err := tx.QueryRowContext(ctx, query, models.OrderStatusPending).Scan(
		&order.ID,
		&order.UserID,
		&order.OrderNumber,
		&order.Status,
		&order.TotalAmount,
		&order.CreatedAt,
		&order.UpdatedAt,
		&order.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, database.ErrOrderNotFound
		}
		return nil, fmt.Errorf("get next pending order: %w", err)
	}

	return order, nil
}
