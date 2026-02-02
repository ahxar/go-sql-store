package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/safar/go-sql-store/internal/database"
	"github.com/safar/go-sql-store/internal/models"
)

func CreateUser(ctx context.Context, db *sql.DB, email, name string) (*models.User, error) {
	user := &models.User{}

	query := `
		INSERT INTO users (email, name, created_at, updated_at, version)
		VALUES ($1, $2, NOW(), NOW(), 1)
		RETURNING id, email, name, created_at, updated_at, version`

	err := db.QueryRowContext(ctx, query, email, name).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Version,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func GetUser(ctx context.Context, db *sql.DB, id int64) (*models.User, error) {
	user := &models.User{}

	query := `
		SELECT id, email, name, created_at, updated_at, version
		FROM users
		WHERE id = $1`

	err := db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, database.ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	return user, nil
}

func ListUsers(ctx context.Context, db *sql.DB, page, pageSize int) (*OffsetPage, error) {
	var total int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	offset := (page - 1) * pageSize
	query := `
		SELECT id, email, name, created_at, updated_at, version
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := db.QueryContext(ctx, query, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.Name,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &OffsetPage{
		Items:      users,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}
