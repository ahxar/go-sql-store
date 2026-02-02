# Database Schema

## Overview
This document describes the database schema for the go-sql-store application, including table structures, relationships, constraints, and indexing strategies.

## Tables

### users
Stores user account information.

```sql
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    version INT NOT NULL DEFAULT 1
);
```

**Indexes:**
- `idx_users_email` - Fast lookups by email (login)
- `idx_users_created_at` - Efficient ordering for pagination

**Design Notes:**
- `version` column supports optimistic locking if needed
- `email` has unique constraint for authentication
- Timestamps track record lifecycle

### products
Stores product catalog with inventory tracking.

```sql
CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    sku VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price DECIMAL(10, 2) NOT NULL CHECK (price >= 0),
    stock_quantity INT NOT NULL DEFAULT 0 CHECK (stock_quantity >= 0),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    version INT NOT NULL DEFAULT 1
);
```

**Indexes:**
- `idx_products_sku` - Fast lookups by SKU
- `idx_products_created_at` - Efficient ordering for pagination
- `idx_products_stock` (partial) - Only indexes products with stock > 0 for inventory queries

**Design Notes:**
- `sku` is unique business identifier
- `price` uses DECIMAL to avoid floating-point precision issues
- `stock_quantity` has CHECK constraint to prevent negative inventory
- `version` enables optimistic locking for concurrent updates
- Partial index on stock_quantity optimizes "available products" queries

### orders
Stores customer orders.

```sql
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    order_number VARCHAR(50) NOT NULL UNIQUE,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    total_amount DECIMAL(10, 2) NOT NULL CHECK (total_amount >= 0),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    version INT NOT NULL DEFAULT 1,
    CONSTRAINT valid_status CHECK (status IN ('pending', 'confirmed', 'shipped', 'delivered', 'cancelled'))
);
```

**Indexes:**
- `idx_orders_user_id` - Fast lookups of user's orders
- `idx_orders_status` - Efficient filtering by status
- `idx_orders_created_at` - Ordering for pagination
- `idx_orders_user_created` (composite) - Optimized for cursor pagination (user_id, created_at DESC, id DESC)

**Design Notes:**
- `order_number` is unique, user-friendly identifier
- `status` constrained to valid values
- `ON DELETE RESTRICT` prevents deleting users with orders
- Composite index supports efficient cursor-based pagination for user orders
- `version` supports optimistic locking

### order_items
Many-to-many relationship between orders and products.

```sql
CREATE TABLE order_items (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity INT NOT NULL CHECK (quantity > 0),
    unit_price DECIMAL(10, 2) NOT NULL CHECK (unit_price >= 0),
    subtotal DECIMAL(10, 2) NOT NULL CHECK (subtotal >= 0),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(order_id, product_id)
);
```

**Indexes:**
- `idx_order_items_order_id` - Fast retrieval of items for an order
- `idx_order_items_product_id` - Fast lookup of orders containing a product

**Design Notes:**
- `ON DELETE CASCADE` automatically removes items when order is deleted
- `ON DELETE RESTRICT` prevents deleting products that are in orders
- `unit_price` denormalized to preserve historical pricing
- `subtotal` denormalized for query performance
- UNIQUE constraint prevents duplicate products in same order

## Relationships

```
users (1) ─────< orders (N)
                   │
                   │ (1)
                   │
                   └──< order_items (N) >──── (N) products
```

## Performance Considerations

### Connection Pooling
- Max open connections: 25
- Max idle connections: 5
- Connection max lifetime: 5 minutes

### Index Strategy
1. **Primary Keys**: Automatic B-tree indexes on all BIGSERIAL id columns
2. **Foreign Keys**: Explicit indexes on all foreign key columns for JOIN performance
3. **Unique Constraints**: Automatic indexes on email, sku, order_number
4. **Cursor Pagination**: Composite index (user_id, created_at DESC, id DESC) enables efficient keyset pagination
5. **Partial Indexes**: Products with stock > 0 for inventory availability queries

### Query Optimization
- Use `FOR UPDATE` carefully to minimize lock duration
- Cursor-based pagination prevents OFFSET scan for deep pages
- Composite indexes support multi-column sorting without table scan

## Constraints Rationale

### CHECK Constraints
- **price >= 0**: Prevents negative prices
- **stock_quantity >= 0**: Prevents negative inventory
- **quantity > 0**: Order items must have positive quantity
- **valid_status**: Ensures data integrity for order workflow

### Foreign Key Actions
- **ON DELETE RESTRICT**: Prevents orphaning critical data (users with orders, products in order history)
- **ON DELETE CASCADE**: Automatically cleans up dependent data (order items when order deleted)

## Migration Strategy

Migrations are applied in numerical order:
1. `001_create_users` - Foundation table
2. `002_create_products` - Independent table
3. `003_create_orders` - Depends on users
4. `004_create_order_items` - Depends on orders and products

Each migration has a corresponding `.down.sql` for rollback with `CASCADE` to handle dependencies.
