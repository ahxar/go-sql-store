# Go SQL Store

A production-grade Go service demonstrating advanced `database/sql` patterns with PostgreSQL. This project showcases transaction management, row-level locking strategies, pagination techniques, and error handling in a real-world e-commerce domain.

## Features

- **Explicit Transaction Management** - Full control over transaction boundaries with retry logic
- **Row-Level Locking Patterns** - FOR UPDATE, NOWAIT, SKIP LOCKED, and optimistic locking
- **Pagination Strategies** - Both cursor-based (keyset) and offset-based pagination
- **Error Classification** - Intelligent retry logic based on PostgreSQL error codes
- **Integration Tests** - Real PostgreSQL via testcontainers for accurate testing

## Quick Start

### Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Make (optional)

### Setup

1. Clone the repository:

```bash
git clone https://github.com/safar/go-sql-store.git
cd go-sql-store
```

2. Copy environment file:

```bash
cp .env.example .env
```

3. Start PostgreSQL:

```bash
make docker-up
```

4. Run migrations:

```bash
make migrate-up
```

5. Start the server:

```bash
make run
```

The API will be available at `http://localhost:8080`.

## API Examples

### Create a User

```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{
    "email": "john@example.com",
    "name": "John Doe"
  }'
```

Response:

```json
{
  "id": 1,
  "email": "john@example.com",
  "name": "John Doe",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z",
  "version": 1
}
```

### Create a Product

```bash
curl -X POST http://localhost:8080/products \
  -H "Content-Type: application/json" \
  -d '{
    "sku": "WIDGET-001",
    "name": "Premium Widget",
    "description": "High-quality widget",
    "price": 29.99,
    "stock": 100
  }'
```

### Create an Order

This demonstrates the full transaction with locking and retry logic:

```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "items": [
      {"product_id": 1, "quantity": 5},
      {"product_id": 2, "quantity": 3}
    ]
  }'
```

The CreateOrder operation:

1. Validates user exists
2. Locks products with FOR UPDATE NOWAIT
3. Checks stock availability
4. Creates order and order items
5. Decrements product stock
6. Automatically retries on deadlocks
7. Uses Serializable isolation level

### List Products (Offset Pagination)

```bash
curl "http://localhost:8080/products?page=1&page_size=20"
```

### List Orders (Cursor Pagination)

```bash
curl "http://localhost:8080/users/1/orders?limit=10"
curl "http://localhost:8080/users/1/orders?limit=10&cursor=<token>"
```

## Architecture

### Project Structure

```
go-sql-store/
├── cmd/api/main.go                    # Application entry point
├── internal/
│   ├── config/config.go               # Configuration management
│   ├── database/
│   │   ├── db.go                      # Connection pooling
│   │   ├── tx.go                      # Transaction helpers + retry ⭐
│   │   └── errors.go                  # Error classification ⭐
│   ├── store/
│   │   ├── users.go                   # User operations
│   │   ├── products.go                # Product operations + locking ⭐
│   │   ├── orders.go                  # Order transactions ⭐
│   │   └── pagination.go              # Pagination utilities ⭐
│   └── models/models.go               # Domain models
├── migrations/                        # SQL migrations
├── tests/integration/                 # Integration tests
└── docs/                              # Documentation

⭐ = Core pattern implementations
```

### Key Patterns

#### 1. Transaction Management (`internal/database/tx.go`)

```go
// Automatic retry on transient errors
err := database.WithRetry(ctx, db, database.TxOptions{
    IsolationLevel: sql.LevelSerializable,
    MaxRetries:     3,
}, func(tx *sql.Tx) error {
    // Transaction work here
    return nil
})
```

#### 2. Row-Level Locking (`internal/store/products.go`)

**Pessimistic (blocks):**

```go
SELECT * FROM products WHERE id = $1 FOR UPDATE
```

**Fail Fast:**

```go
SELECT * FROM products WHERE id = $1 FOR UPDATE NOWAIT
```

**Job Queue:**

```go
SELECT * FROM orders WHERE status = 'pending'
ORDER BY created_at
FOR UPDATE SKIP LOCKED
LIMIT 1
```

**Optimistic:**

```go
UPDATE products
SET stock = $1, version = version + 1
WHERE id = $2 AND version = $3
```

#### 3. Cursor Pagination (`internal/store/pagination.go`)

```go
SELECT id, created_at
FROM orders
WHERE (created_at, id) < ($1, $2)  -- Composite comparison
ORDER BY created_at DESC, id DESC
LIMIT 10
```

Benefits:

- Constant performance (no OFFSET scan)
- Stable results under concurrent writes
- Scales to millions of rows

#### 4. Error Classification (`internal/database/errors.go`)

```go
switch pqErr.Code {
case "40001": // serialization_failure
    return ErrorClassSerialization  // Retry
case "40P01": // deadlock_detected
    return ErrorClassDeadlock       // Retry
case "23505": // unique_violation
    return ErrorClassPermanent      // Don't retry
}
```

## Testing

### Run Integration Tests

```bash
make test
```

Tests use real PostgreSQL via testcontainers:

```go
func TestConcurrentOrderCreation(t *testing.T) {
    db, cleanup := setupTestDB(t)
    defer cleanup()

    // 10 concurrent goroutines trying to order same product
    // Verifies: locking, stock consistency, retry logic
}
```

### Test Coverage

- Concurrent stock reservation
- Deadlock detection and retry
- Optimistic locking failures
- Cursor pagination
- Transaction rollback on errors

## Makefile Commands

| Command             | Description                |
| ------------------- | -------------------------- |
| `make docker-up`    | Start PostgreSQL container |
| `make docker-down`  | Stop PostgreSQL container  |
| `make migrate-up`   | Run database migrations    |
| `make migrate-down` | Rollback migrations        |
| `make run`          | Start the API server       |
| `make test`         | Run integration tests      |
| `make clean`        | Clean build artifacts      |

## Configuration

Environment variables (`.env`):

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/sqlstore?sslmode=disable
DATABASE_MAX_OPEN_CONNS=25
DATABASE_MAX_IDLE_CONNS=5
DATABASE_CONN_MAX_LIFETIME=5m

SERVER_PORT=8080
SERVER_READ_TIMEOUT=10s
SERVER_WRITE_TIMEOUT=10s
```

## Documentation

- [Schema Documentation](docs/schema.md) - Database design and indexing
- [Patterns Guide](docs/patterns.md) - Detailed pattern explanations
- [Transaction Lifecycle](docs/transaction_lifecycle.puml) - PlantUML diagram

## Database Schema

```
users (1) ─────< orders (N)
                   │
                   │ (1)
                   │
                   └──< order_items (N) >──── (N) products
```

Key features:

- Foreign key constraints with appropriate ON DELETE actions
- CHECK constraints for data integrity
- Composite indexes for cursor pagination
- Partial indexes for performance
- Version columns for optimistic locking

## Performance Notes

### Connection Pool

- Max open: 25 connections
- Max idle: 5 connections
- Lifetime: 5 minutes

### Indexes

All queries use indexes:

- Cursor pagination uses composite index (user_id, created_at DESC, id DESC)
- Foreign keys have indexes for JOINs
- Unique constraints automatically indexed

### Lock Duration

Keep transactions short:

- Lock products → Check stock → Update → Commit
- Total lock time: ~10ms
- Use NOWAIT to fail fast in user-facing operations

## Troubleshooting

### Check Active Connections

```sql
SELECT * FROM pg_stat_activity WHERE state != 'idle';
```

### Check Locks

```sql
SELECT * FROM pg_locks WHERE NOT granted;
```

### Check Deadlocks

```bash
# In PostgreSQL logs
ERROR: deadlock detected
DETAIL: Process 1234 waits for ShareLock on transaction 5678
```

Application automatically retries deadlocks with exponential backoff.

## Contributing

This is a reference implementation demonstrating patterns. Feel free to:

- Study the code
- Use patterns in your projects
- Adapt to your needs

## License

MIT License

## Acknowledgments

Built with:

- [database/sql](https://pkg.go.dev/database/sql) - Go standard library
- [lib/pq](https://github.com/lib/pq) - PostgreSQL driver
- [testcontainers-go](https://github.com/testcontainers/testcontainers-go) - Integration testing
- [shopspring/decimal](https://github.com/shopspring/decimal) - Precise decimal arithmetic
