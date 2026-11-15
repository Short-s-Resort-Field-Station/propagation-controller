package storage

import (
	"database/sql"
	"time"
)

// PushSubscription is one browser that asked for notifications.
type PushSubscription struct {
	ID        int64   `json:"id"`
	Username  string  `json:"username"`
	Endpoint  string  `json:"endpoint"`
	P256dh    string  `json:"-"`
	Auth      string  `json:"-"`
	PrefsJSON string  `json:"prefs_json"`
	Label     string  `json:"label"`
	CreatedAt string  `json:"created_at"`
	LastOKAt  *string `json:"last_ok_at,omitempty"`
	Failures  int     `json:"failures"`
}

type PushRepo struct{ db *sql.DB }

func NewPushRepo(db *sql.DB) *PushRepo { return &PushRepo{db: db} }

// Upsert keys on the endpoint: re-subscribing from the same browser updates
// the keys and prefs in place and keeps the row with the signed-in user.
func (r *PushRepo) Upsert(sub PushSubscription) error {
	_, err := r.db.Exec(`INSERT INTO push_subscriptions (username, endpoint, p256dh, auth, prefs_json, label)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET username = excluded.username, p256dh = excluded.p256dh,
			auth = excluded.auth, prefs_json = excluded.prefs_json, label = excluded.label, failures = 0`,
		sub.Username, sub.Endpoint, sub.P256dh, sub.Auth, sub.PrefsJSON, sub.Label)
	return err
}

func (r *PushRepo) GetByEndpoint(endpoint string) (*PushSubscription, error) {
	rows, err := r.db.Query(`SELECT id, username, endpoint, p256dh, auth, prefs_json, label, created_at, last_ok_at, failures
		FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	subs, err := scanPush(rows)
	if err != nil || len(subs) == 0 {
		return nil, err
	}
	return &subs[0], nil
}

func (r *PushRepo) List() ([]PushSubscription, error) {
	rows, err := r.db.Query(`SELECT id, username, endpoint, p256dh, auth, prefs_json, label, created_at, last_ok_at, failures
		FROM push_subscriptions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPush(rows)
}

func (r *PushRepo) Delete(endpoint string) error {
	_, err := r.db.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}

// DeleteByUser drops every device of an operator (used when the account goes).
func (r *PushRepo) DeleteByUser(username string) error {
	_, err := r.db.Exec(`DELETE FROM push_subscriptions WHERE username = ?`, username)
	return err
}

func (r *PushRepo) MarkOK(endpoint string) error {
	_, err := r.db.Exec(`UPDATE push_subscriptions SET last_ok_at = ?, failures = 0 WHERE endpoint = ?`,
		time.Now().UTC().Format(time.RFC3339), endpoint)
	return err
}

func (r *PushRepo) MarkFailure(endpoint string) error {
	_, err := r.db.Exec(`UPDATE push_subscriptions SET failures = failures + 1 WHERE endpoint = ?`, endpoint)
	return err
}

func scanPush(rows *sql.Rows) ([]PushSubscription, error) {
	var out []PushSubscription
	for rows.Next() {
		var s PushSubscription
		if err := rows.Scan(&s.ID, &s.Username, &s.Endpoint, &s.P256dh, &s.Auth, &s.PrefsJSON, &s.Label, &s.CreatedAt, &s.LastOKAt, &s.Failures); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
