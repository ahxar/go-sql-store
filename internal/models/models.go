package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int       `json:"version"`
}

type Product struct {
	ID            int64           `json:"id"`
	SKU           string          `json:"sku"`
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Price         decimal.Decimal `json:"price"`
	StockQuantity int             `json:"stock_quantity"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Version       int             `json:"version"`
}

type Order struct {
	ID          int64           `json:"id"`
	UserID      int64           `json:"user_id"`
	OrderNumber string          `json:"order_number"`
	Status      string          `json:"status"`
	TotalAmount decimal.Decimal `json:"total_amount"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Version     int             `json:"version"`
	Items       []OrderItem     `json:"items,omitempty"`
}

type OrderItem struct {
	ID        int64           `json:"id"`
	OrderID   int64           `json:"order_id"`
	ProductID int64           `json:"product_id"`
	Quantity  int             `json:"quantity"`
	UnitPrice decimal.Decimal `json:"unit_price"`
	Subtotal  decimal.Decimal `json:"subtotal"`
	CreatedAt time.Time       `json:"created_at"`
}

const (
	OrderStatusPending   = "pending"
	OrderStatusConfirmed = "confirmed"
	OrderStatusShipped   = "shipped"
	OrderStatusDelivered = "delivered"
	OrderStatusCancelled = "cancelled"
)
