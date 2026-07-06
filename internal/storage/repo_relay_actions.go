package storage

import (
	"database/sql"
	"time"
)

type RelayAction struct {
	ID             int64     `json:"id"`
	RelayChannel   int       `json:"relay_channel"`
	DeviceKey      *string   `json:"device_key,omitempty"`
	PreviousState  *string   `json:"previous_state,omitempty"`
	RequestedState string    `json:"requested_state"`
	ActualState    *string   `json:"actual_state,omitempty"`
	Actor          string    `json:"actor"`
	Reason         *string   `json:"reason,omitempty"`
	Success        bool      `json:"success"`
	ErrorMessage   *string   `json:"error_message,omitempty"`
	OccurredAt     time.Time `json:"occurred_at"`
}

type RelayActionRepo struct{ db *sql.DB }

func NewRelayActionRepo(db *sql.DB) *RelayActionRepo { return &RelayActionRepo{db: db} }

func (r *RelayActionRepo) Insert(a RelayAction) error {
	_, err := r.db.Exec(
		`INSERT INTO relay_actions
		 (relay_channel, device_key, previous_state, requested_state, actual_state, actor, reason, success, error_message, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.RelayChannel, a.DeviceKey, a.PreviousState, a.RequestedState, a.ActualState,
		a.Actor, a.Reason, a.Success, a.ErrorMessage, time.Now().UTC(),
	)
	return err
}

func (r *RelayActionRepo) List(from, to *time.Time, limit int) ([]RelayAction, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, relay_channel, device_key, previous_state, requested_state, actual_state,
	                 actor, reason, success, error_message, occurred_at
	          FROM relay_actions WHERE 1=1`
	args := []any{}
	if from != nil {
		query += " AND occurred_at >= ?"
		args = append(args, from.UTC())
	}
	if to != nil {
		query += " AND occurred_at <= ?"
		args = append(args, to.UTC())
	}
	query += " ORDER BY occurred_at DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []RelayAction
	for rows.Next() {
		var a RelayAction
		if err := rows.Scan(&a.ID, &a.RelayChannel, &a.DeviceKey, &a.PreviousState, &a.RequestedState,
			&a.ActualState, &a.Actor, &a.Reason, &a.Success, &a.ErrorMessage, &a.OccurredAt); err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}
