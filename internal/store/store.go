// Package store contiene el acceso a datos contra el esquema PostgreSQL
// existente (tablas books, authors, publishers, users).
package store

import (
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

var (
	ErrNotFound     = errors.New("store: record not found")
	ErrEditConflict = errors.New("store: edit conflict")
	ErrHasBooks     = errors.New("store: record still has books associated")
	ErrDuplicate    = errors.New("store: duplicate record")
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
