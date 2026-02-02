package integration

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/safar/go-sql-store/internal/database"
	"github.com/safar/go-sql-store/internal/store"
	"github.com/shopspring/decimal"
)

func TestConcurrentStockReservation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	product, err := store.CreateProduct(ctx, db, "TEST-001", "Test Product", "Test", decimal.NewFromInt(100), 10)
	if err != nil {
		t.Fatalf("Create product: %v", err)
	}

	concurrency := 5
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := database.WithTransaction(ctx, db, database.DefaultTxOptions(), func(tx *sql.Tx) error {
				_, err := store.ReserveStock(ctx, tx, product.ID, 2)
				if err != nil {
					return err
				}

				return store.DecrementStock(ctx, tx, product.ID, 2)
			})

			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	successCount := concurrency
	for err := range errors {
		if err != nil {
			successCount--
		}
	}

	finalProduct, err := store.GetProduct(ctx, db, product.ID)
	if err != nil {
		t.Fatalf("Get product: %v", err)
	}

	expectedStock := 10 - (successCount * 2)
	if finalProduct.StockQuantity != expectedStock {
		t.Errorf("Expected stock %d, got %d", expectedStock, finalProduct.StockQuantity)
	}
}

func TestOptimisticLocking(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	product, err := store.CreateProduct(ctx, db, "TEST-002", "Test Product 2", "Test", decimal.NewFromInt(100), 50)
	if err != nil {
		t.Fatalf("Create product: %v", err)
	}

	err = store.UpdateStockOptimistic(ctx, db, product.ID, 40, product.Version)
	if err != nil {
		t.Fatalf("First update should succeed: %v", err)
	}

	err = store.UpdateStockOptimistic(ctx, db, product.ID, 30, product.Version)
	if err != database.ErrOptimisticLockFailed {
		t.Errorf("Expected optimistic lock failure, got: %v", err)
	}
}

func TestReserveStockNoWait(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	product, err := store.CreateProduct(ctx, db, "TEST-003", "Test Product 3", "Test", decimal.NewFromInt(100), 20)
	if err != nil {
		t.Fatalf("Create product: %v", err)
	}

	tx1, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Begin tx1: %v", err)
	}
	defer func() { _ = tx1.Rollback() }()

	_, err = store.ReserveStock(ctx, tx1, product.ID, 5)
	if err != nil {
		t.Fatalf("Reserve stock in tx1: %v", err)
	}

	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Begin tx2: %v", err)
	}
	defer func() { _ = tx2.Rollback() }()

	_, err = store.ReserveStockNoWait(ctx, tx2, product.ID, 3)
	if err != database.ErrLockTimeout {
		t.Errorf("Expected lock timeout, got: %v", err)
	}
}
