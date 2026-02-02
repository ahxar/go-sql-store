# Database Patterns Guide

This document explains the advanced database/sql patterns implemented in go-sql-store.

## Table of Contents
1. [Transaction Management](#transaction-management)
2. [Row-Level Locking](#row-level-locking)
3. [Pagination Strategies](#pagination-strategies)
4. [Error Classification](#error-classification)
5. [Testing Patterns](#testing-patterns)

## Transaction Management

### Explicit Transactions

Functions accept `*sql.Tx` when they need to participate in a transaction:

```go
func ReserveStock(ctx context.Context, tx *sql.Tx, productID int64, quantity int) error
```

The caller controls transaction boundaries:

```go
tx, _ := db.BeginTx(ctx, nil)
defer tx.Rollback()

if err := ReserveStock(ctx, tx, productID, qty); err != nil {
    return err
}

if err := DecrementStock(ctx, tx, productID, qty); err != nil {
    return err
}

return tx.Commit()
```

### Helper Functions

**WithTransaction** - Basic transaction wrapper:

```go
err := database.WithTransaction(ctx, db, opts, func(tx *sql.Tx) error {
    // Automatic rollback on error
    // Automatic commit on success
    return doWork(tx)
})
```

**WithRetry** - Transaction with automatic retry logic:

```go
err := database.WithRetry(ctx, db, database.TxOptions{
    IsolationLevel: sql.LevelSerializable,
    MaxRetries:     3,
}, func(tx *sql.Tx) error {
    // Retries on deadlocks and serialization failures
    // Exponential backoff with jitter
    return doWork(tx)
})
```

### Isolation Levels

| Level | Use Case | Trade-offs |
|-------|----------|------------|
| `ReadCommitted` (default) | Most operations | Good performance, read skew possible |
| `RepeatableRead` | Consistent reads within transaction | Slower, can cause serialization failures |
| `Serializable` | Critical financial operations | Slowest, highest consistency, more conflicts |

**Example:**
```go
opts := database.TxOptions{
    IsolationLevel: sql.LevelSerializable,
    MaxRetries:     3,
}
```

## Row-Level Locking

### 1. Pessimistic Locking (FOR UPDATE)

Blocks other transactions until lock is released.

```go
func ReserveStock(ctx context.Context, tx *sql.Tx, productID int64) error {
    var stock int
    err := tx.QueryRowContext(ctx,
        `SELECT stock_quantity FROM products WHERE id = $1 FOR UPDATE`,
        productID).Scan(&stock)
    // Lock held until transaction commits or rolls back
}
```

**Use when:**
- High contention expected
- You need guaranteed consistency
- Transaction duration is short

**Example:** Order processing where multiple workers might try to reserve the same product.

### 2. Fail Fast (FOR UPDATE NOWAIT)

Returns error immediately if row is locked.

```go
func ReserveStockNoWait(ctx context.Context, tx *sql.Tx, productID int64) error {
    var stock int
    err := tx.QueryRowContext(ctx,
        `SELECT stock_quantity FROM products WHERE id = $1 FOR UPDATE NOWAIT`,
        productID).Scan(&stock)

    if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "55P03" {
        return ErrLockTimeout
    }
}
```

**Use when:**
- User-facing operations that can't wait
- You want to fail fast and retry later
- Alternative action available on lock failure

**Example:** Interactive checkout - show "Product locked, try again" instead of making user wait.

### 3. Skip Locked Rows (SKIP LOCKED)

Skips over locked rows, gets next available.

```go
func GetNextPendingOrder(ctx context.Context, tx *sql.Tx) (*Order, error) {
    var order Order
    err := tx.QueryRowContext(ctx,
        `SELECT id, order_number FROM orders
         WHERE status = 'pending'
         ORDER BY created_at
         FOR UPDATE SKIP LOCKED
         LIMIT 1`,
    ).Scan(&order.ID, &order.OrderNumber)
}
```

**Use when:**
- Worker queue pattern
- Multiple workers processing items
- Don't care which specific item you get

**Example:** Background job processors picking up pending orders - if one is locked, skip to the next.

### 4. Optimistic Locking

Uses version number to detect concurrent modifications.

```go
func UpdateStockOptimistic(ctx context.Context, db *sql.DB, productID int64, newStock, version int) error {
    result, err := db.ExecContext(ctx,
        `UPDATE products
         SET stock_quantity = $1, version = version + 1, updated_at = NOW()
         WHERE id = $2 AND version = $3`,
        newStock, productID, version)

    if rowsAffected == 0 {
        return ErrOptimisticLockFailed
    }
}
```

**Use when:**
- Low contention expected
- Long-running operations (user editing form)
- Want to avoid holding database locks

**Example:** Admin updating product details - read version, user edits, save with version check.

## Pagination Strategies

### Cursor-Based (Keyset) Pagination

**Advantages:**
- Constant performance regardless of page depth
- Stable results even with concurrent writes
- No duplicate/missing items when data changes

**Implementation:**

```go
type OrderCursor struct {
    CreatedAt time.Time
    ID        int64
}

query := `
    SELECT id, order_number, created_at
    FROM orders
    WHERE user_id = $1
      AND (created_at, id) < ($2, $3)  -- Composite comparison
    ORDER BY created_at DESC, id DESC
    LIMIT $4`
```

**Index requirement:**
```sql
CREATE INDEX idx_orders_user_created
ON orders(user_id, created_at DESC, id DESC);
```

**Use when:**
- Infinite scroll / "Load More" UI
- API pagination for external consumers
- Real-time data feeds
- Large datasets

### Offset-Based Pagination

**Advantages:**
- Simple to implement
- Direct page access (jump to page 5)
- Total count available

**Disadvantages:**
- Slow for deep pages (OFFSET 10000)
- Duplicate/missing items with concurrent writes

**Implementation:**

```go
offset := (page - 1) * pageSize

query := `
    SELECT id, name FROM products
    ORDER BY created_at DESC
    LIMIT $1 OFFSET $2`
```

**Use when:**
- Admin panels with page numbers
- Small datasets
- Stable data (few writes)
- Total count needed for UI

## Error Classification

### Why Classify Errors?

Not all database errors are created equal. Some are transient (deadlocks), others are permanent (constraint violations).

### Error Classes

```go
type ErrorClass int

const (
    ErrorClassPermanent     // Don't retry
    ErrorClassTransient     // Retry with backoff
    ErrorClassDeadlock      // Retry immediately
    ErrorClassSerialization // Retry with backoff
)
```

### PostgreSQL Error Codes

| Code | Name | Class | Action |
|------|------|-------|--------|
| 40001 | serialization_failure | Serialization | Retry |
| 40P01 | deadlock_detected | Deadlock | Retry |
| 55P03 | lock_not_available | Transient | Retry |
| 23505 | unique_violation | Permanent | Fail |
| 23503 | foreign_key_violation | Permanent | Fail |

### Classification Logic

```go
func ClassifyError(err error) ErrorClass {
    var pqErr *pq.Error
    if errors.As(err, &pqErr) {
        switch pqErr.Code {
        case "40001":
            return ErrorClassSerialization
        case "40P01":
            return ErrorClassDeadlock
        case "55P03":
            return ErrorClassTransient
        default:
            return ErrorClassPermanent
        }
    }
    return ErrorClassPermanent
}
```

### Retry Strategy

**Exponential Backoff with Jitter:**

```go
backoff := 50 * time.Millisecond

for attempt := 0; attempt <= maxRetries; attempt++ {
    err := tryOperation()

    if err == nil {
        return nil
    }

    if ClassifyError(err) == ErrorClassPermanent {
        return err  // Don't retry
    }

    if attempt < maxRetries {
        jitter := rand.Int63n(int64(backoff / 4))
        time.Sleep(backoff + jitter)
        backoff *= 2
    }
}
```

**Why Jitter?**
- Prevents thundering herd
- Spreads out retry attempts
- Better for high-concurrency scenarios

## Testing Patterns

### Integration Testing with testcontainers

```go
func setupTestDB(t *testing.T) (*sql.DB, func()) {
    ctx := context.Background()

    req := testcontainers.ContainerRequest{
        Image: "postgres:14-alpine",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_USER": "testuser",
            "POSTGRES_PASSWORD": "testpass",
            "POSTGRES_DB": "testdb",
        },
        WaitingFor: wait.ForLog("ready to accept connections").
            WithOccurrence(2),
    }

    container, _ := testcontainers.GenericContainer(ctx, ...)

    // Real PostgreSQL instance for each test
    db, _ := sql.Open("postgres", dsn)

    cleanup := func() {
        db.Close()
        container.Terminate(ctx)
    }

    return db, cleanup
}
```

### Concurrent Test Pattern

```go
func TestConcurrentOperation(t *testing.T) {
    db, cleanup := setupTestDB(t)
    defer cleanup()

    concurrency := 10
    var wg sync.WaitGroup
    errors := make(chan error, concurrency)

    for i := 0; i < concurrency; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            err := performOperation(db)
            errors <- err
        }()
    }

    wg.Wait()
    close(errors)

    // Verify results
    for err := range errors {
        if err != nil {
            t.Errorf("Operation failed: %v", err)
        }
    }
}
```

## Best Practices

1. **Keep Transactions Short** - Lock duration = transaction duration
2. **Lock in Consistent Order** - Prevents deadlocks (always product_id ASC)
3. **Use Context Timeouts** - Prevent hanging transactions
4. **Choose Right Isolation Level** - Balance consistency vs performance
5. **Index Pagination Columns** - Especially for cursor pagination
6. **Monitor Lock Waits** - Use `pg_stat_activity` and `pg_locks`
7. **Test Concurrent Scenarios** - Real PostgreSQL in tests
8. **Classify Errors** - Retry only transient failures
