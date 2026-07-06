package storage

import (
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"` // admin | operator | viewer
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserRepo struct{ db *sql.DB }

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Count() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (r *UserRepo) Insert(username, passwordHash, role string) error {
	_, err := r.db.Exec(
		`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`,
		username, passwordHash, role)
	return err
}

func (r *UserRepo) GetByUsername(username string) (*User, error) {
	var u User
	err := r.db.QueryRow(
		`SELECT id, username, password_hash, role, enabled, created_at
		 FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) List() ([]User, error) {
	rows, err := r.db.Query(
		`SELECT id, username, password_hash, role, enabled, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserRepo) SetRole(username, role string) error {
	_, err := r.db.Exec(`UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, role, username)
	return err
}

func (r *UserRepo) SetEnabled(username string, enabled bool) error {
	_, err := r.db.Exec(`UPDATE users SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, enabled, username)
	return err
}

func (r *UserRepo) SetPassword(username, passwordHash string) error {
	_, err := r.db.Exec(`UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, passwordHash, username)
	return err
}

func (r *UserRepo) Delete(username string) error {
	_, err := r.db.Exec(`DELETE FROM users WHERE username = ?`, username)
	return err
}

// --- sessions ---

type SessionRepo struct{ db *sql.DB }

func NewSessionRepo(db *sql.DB) *SessionRepo { return &SessionRepo{db: db} }

func (r *SessionRepo) Insert(userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, tokenHash, expiresAt.UTC())
	return err
}

// ResolveUser returns the enabled user for a live session token hash.
func (r *SessionRepo) ResolveUser(tokenHash string) (*User, error) {
	var u User
	err := r.db.QueryRow(
		`SELECT u.id, u.username, u.password_hash, u.role, u.enabled, u.created_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = ? AND s.expires_at > ? AND u.enabled = 1`,
		tokenHash, time.Now().UTC()).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *SessionRepo) Delete(tokenHash string) error {
	_, err := r.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (r *SessionRepo) DeleteExpired() error {
	_, err := r.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().UTC())
	return err
}

func (r *SessionRepo) DeleteForUser(userID int64) error {
	_, err := r.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}
