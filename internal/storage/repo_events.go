package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Event struct {
	ID          int64     `json:"id"`
	Level       string    `json:"level"`
	EventType   string    `json:"event_type"`
	Message     string    `json:"message"`
	DetailsJSON *string   `json:"details_json,omitempty"`
	Actor       *string   `json:"actor,omitempty"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type EventFilter struct {
	Level     string
	EventType string
	From      *time.Time
	To        *time.Time
	Limit     int
}

type EventRepo struct{ db *sql.DB }

func NewEventRepo(db *sql.DB) *EventRepo { return &EventRepo{db: db} }

func (r *EventRepo) Insert(level, eventType, message string, detailsJSON, actor *string) error {
	_, err := r.db.Exec(
		`INSERT INTO system_events (level, event_type, message, details_json, actor, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		level, eventType, message, detailsJSON, actor, time.Now().UTC(),
	)
	return err
}

func (r *EventRepo) List(f EventFilter) ([]Event, error) {
	where := []string{"1=1"}
	args := []any{}
	if f.Level != "" {
		where = append(where, "level = ?")
		args = append(args, f.Level)
	}
	if f.EventType != "" {
		where = append(where, "event_type = ?")
		args = append(args, f.EventType)
	}
	if f.From != nil {
		where = append(where, "occurred_at >= ?")
		args = append(args, f.From.UTC())
	}
	if f.To != nil {
		where = append(where, "occurred_at <= ?")
		args = append(args, f.To.UTC())
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)

	query := fmt.Sprintf(
		`SELECT id, level, event_type, message, details_json, actor, occurred_at
		 FROM system_events WHERE %s ORDER BY occurred_at DESC, id DESC LIMIT ?`,
		strings.Join(where, " AND "),
	)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Level, &e.EventType, &e.Message, &e.DetailsJSON, &e.Actor, &e.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
