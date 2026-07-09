// Package store contiene el acceso a datos contra el esquema PostgreSQL
// existente (tablas books, authors, publishers, users).
package store

import (
	"database/sql"
	"errors"
)

var ErrNotFound = errors.New("store: record not found")

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}
