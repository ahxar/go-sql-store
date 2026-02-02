package integration

import (
	"context"
	"sync"
	"testing"

	"github.com/safar/go-sql-store/internal/database"
	"github.com/safar/go-sql-store/internal/store"
	"github.com/shopspring/decimal"
)

func TestCreateOrder(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	user, err := store.CreateUser(ctx, db, "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}

	product1, err := store.CreateProduct(ctx, db, "TEST-ORD-001", "Product 1", "Test", decimal.NewFromInt(100), 50)
	if err != nil {
		t.Fatalf("Create product 1: %v", err)
	}

	product2, err := store.CreateProduct(ctx, db, "TEST-ORD-002", "Product 2", "Test", decimal.NewFromInt(200), 30)
	if err != nil {
		t.Fatalf("Create product 2: %v", err)
	}

	order, err := store.CreateOrder(ctx, db, store.CreateOrderRequest{
		UserID: user.ID,
		Items: []store.OrderItemRequest{
			{ProductID: product1.ID, Quantity: 5},
			{ProductID: product2.ID, Quantity: 3},
		},
	})
	if err != nil {
		t.Fatalf("Create order: %v", err)
	}

	if order.ID == 0 {
		t.Error("Order ID should not be 0")
	}

	expectedTotal := decimal.NewFromInt(100).Mul(decimal.NewFromInt(5)).
		Add(decimal.NewFromInt(200).Mul(decimal.NewFromInt(3)))

	if !order.TotalAmount.Equal(expectedTotal) {
		t.Errorf("Expected total %s, got %s", expectedTotal, order.TotalAmount)
	}

	product1After, err := store.GetProduct(ctx, db, product1.ID)
	if err != nil {
		t.Fatalf("Get product 1: %v", err)
	}
	if product1After.StockQuantity != 45 {
		t.Errorf("Expected product 1 stock 45, got %d", product1After.StockQuantity)
	}

	product2After, err := store.GetProduct(ctx, db, product2.ID)
	if err != nil {
		t.Fatalf("Get product 2: %v", err)
	}
	if product2After.StockQuantity != 27 {
		t.Errorf("Expected product 2 stock 27, got %d", product2After.StockQuantity)
	}
}

func TestCreateOrderInsufficientStock(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	user, err := store.CreateUser(ctx, db, "test2@example.com", "Test User 2")
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}

	product, err := store.CreateProduct(ctx, db, "TEST-ORD-003", "Product 3", "Test", decimal.NewFromInt(100), 5)
	if err != nil {
		t.Fatalf("Create product: %v", err)
	}

	_, err = store.CreateOrder(ctx, db, store.CreateOrderRequest{
		UserID: user.ID,
		Items: []store.OrderItemRequest{
			{ProductID: product.ID, Quantity: 10},
		},
	})

	if err != database.ErrInsufficientStock {
		t.Errorf("Expected insufficient stock error, got: %v", err)
	}

	productAfter, err := store.GetProduct(ctx, db, product.ID)
	if err != nil {
		t.Fatalf("Get product: %v", err)
	}
	if productAfter.StockQuantity != 5 {
		t.Errorf("Stock should remain unchanged at 5, got %d", productAfter.StockQuantity)
	}
}

func TestConcurrentOrderCreation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	user, err := store.CreateUser(ctx, db, "test3@example.com", "Test User 3")
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}

	product, err := store.CreateProduct(ctx, db, "TEST-ORD-004", "Product 4", "Test", decimal.NewFromInt(100), 20)
	if err != nil {
		t.Fatalf("Create product: %v", err)
	}

	concurrency := 10
	var wg sync.WaitGroup
	results := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			_, err := store.CreateOrder(ctx, db, store.CreateOrderRequest{
				UserID: user.ID,
				Items: []store.OrderItemRequest{
					{ProductID: product.ID, Quantity: 2},
				},
			})

			results <- err
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	insufficientStockCount := 0

	for err := range results {
		switch err {
		case nil:
			successCount++
		case database.ErrInsufficientStock:
			insufficientStockCount++
		default:
			t.Logf("Unexpected error: %v", err)
		}
	}

	expectedSuccess := 10
	if successCount != expectedSuccess {
		t.Errorf("Expected %d successful orders, got %d", expectedSuccess, successCount)
	}

	productAfter, err := store.GetProduct(ctx, db, product.ID)
	if err != nil {
		t.Fatalf("Get product: %v", err)
	}

	expectedStock := 20 - (successCount * 2)
	if productAfter.StockQuantity != expectedStock {
		t.Errorf("Expected final stock %d, got %d", expectedStock, productAfter.StockQuantity)
	}
}

func TestListOrdersCursor(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	user, err := store.CreateUser(ctx, db, "test4@example.com", "Test User 4")
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}

	product, err := store.CreateProduct(ctx, db, "TEST-ORD-005", "Product 5", "Test", decimal.NewFromInt(100), 100)
	if err != nil {
		t.Fatalf("Create product: %v", err)
	}

	for i := 0; i < 15; i++ {
		_, err := store.CreateOrder(ctx, db, store.CreateOrderRequest{
			UserID: user.ID,
			Items: []store.OrderItemRequest{
				{ProductID: product.ID, Quantity: 1},
			},
		})
		if err != nil {
			t.Fatalf("Create order %d: %v", i, err)
		}
	}

	page1, err := store.ListOrdersCursor(ctx, db, user.ID, "", 10)
	if err != nil {
		t.Fatalf("List orders page 1: %v", err)
	}

	if !page1.HasMore {
		t.Error("Page 1 should have more results")
	}

	if page1.NextCursor == "" {
		t.Error("Page 1 should have a next cursor")
	}

	page2, err := store.ListOrdersCursor(ctx, db, user.ID, page1.NextCursor, 10)
	if err != nil {
		t.Fatalf("List orders page 2: %v", err)
	}

	if page2.HasMore {
		t.Error("Page 2 should not have more results")
	}
}
