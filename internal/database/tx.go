package database

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"time"
)

type TxOptions struct {
	IsolationLevel sql.IsolationLevel
	ReadOnly       bool
	MaxRetries     int
}

func DefaultTxOptions() TxOptions {
	return TxOptions{
		IsolationLevel: sql.LevelReadCommitted,
		ReadOnly:       false,
		MaxRetries:     3,
	}
}

func WithTransaction(ctx context.Context, db *sql.DB, opts TxOptions, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{
		Isolation: opts.IsolationLevel,
		ReadOnly:  opts.ReadOnly,
	})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func WithRetry(ctx context.Context, db *sql.DB, opts TxOptions, fn func(*sql.Tx) error) error {
	var lastErr error
	backoff := 50 * time.Millisecond

	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		tx, err := db.BeginTx(ctx, &sql.TxOptions{
			Isolation: opts.IsolationLevel,
			ReadOnly:  opts.ReadOnly,
		})
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}

		err = fn(tx)
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
			}

			errClass := ClassifyError(err)
			if errClass == ErrorClassPermanent {
				return err
			}

			if attempt == opts.MaxRetries {
				return fmt.Errorf("max retries (%d) exceeded: %w", opts.MaxRetries, err)
			}

			lastErr = err

			jitter := time.Duration(rand.Int63n(int64(backoff / 4)))
			sleepDuration := backoff + jitter

			select {
			case <-time.After(sleepDuration):
			case <-ctx.Done():
				return ctx.Err()
			}

			backoff *= 2
			continue
		}

		if err := tx.Commit(); err != nil {
			errClass := ClassifyError(err)
			if errClass == ErrorClassPermanent {
				return fmt.Errorf("commit transaction: %w", err)
			}

			if attempt == opts.MaxRetries {
				return fmt.Errorf("max retries (%d) exceeded on commit: %w", opts.MaxRetries, err)
			}

			lastErr = err

			jitter := time.Duration(rand.Int63n(int64(backoff / 4)))
			sleepDuration := backoff + jitter

			select {
			case <-time.After(sleepDuration):
			case <-ctx.Done():
				return ctx.Err()
			}

			backoff *= 2
			continue
		}

		return nil
	}

	return lastErr
}
