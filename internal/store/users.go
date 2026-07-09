package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID           int64
	Name         string
	Email        string
	PasswordHash []byte
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var u User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, email, password_hash
		FROM users
		WHERE email = $1 AND activated = TRUE`, email,
	).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}
