package database

import (
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

type ErrorClass int

const (
	ErrorClassPermanent ErrorClass = iota
	ErrorClassTransient
	ErrorClassDeadlock
	ErrorClassSerialization
)

func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassPermanent
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "40001":
			return ErrorClassSerialization
		case "40P01":
			return ErrorClassDeadlock
		case "55P03":
			return ErrorClassTransient
		case "23505", "23503", "23502", "23514":
			return ErrorClassPermanent
		}
	}

	if errors.Is(err, sql.ErrNoRows) {
		return ErrorClassPermanent
	}

	return ErrorClassPermanent
}

func IsRetryable(err error) bool {
	class := ClassifyError(err)
	return class == ErrorClassTransient ||
		class == ErrorClassDeadlock ||
		class == ErrorClassSerialization
}

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrProductNotFound     = errors.New("product not found")
	ErrOrderNotFound       = errors.New("order not found")
	ErrInsufficientStock   = errors.New("insufficient stock")
	ErrOptimisticLockFailed = errors.New("optimistic lock failed")
	ErrLockTimeout         = errors.New("lock timeout")
)
