package storage

import (
	"database/sql"
	"errors"
	"time"
)

// SpeciesProfile is a per-species, per-stage saved settings preset (change 3).
// SettingsJSON is stored and returned verbatim — the moisture-control settings
// plus EC/temp target ranges — so the schema stays stable as the settings shape
// evolves. Validation lives in the API layer.
type SpeciesProfile struct {
	ID           int64     `json:"id"`
	SpeciesID    string    `json:"species_id"`
	Stage        string    `json:"stage"`
	Name         string    `json:"name"`
	SettingsJSON string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type SpeciesProfileRepo struct{ db *sql.DB }

func NewSpeciesProfileRepo(db *sql.DB) *SpeciesProfileRepo { return &SpeciesProfileRepo{db: db} }

const speciesProfileCols = `id, species_id, stage, name, settings_json, created_at, updated_at`

func scanProfile(sc interface{ Scan(...any) error }) (*SpeciesProfile, error) {
	var p SpeciesProfile
	if err := sc.Scan(&p.ID, &p.SpeciesID, &p.Stage, &p.Name, &p.SettingsJSON,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns profiles for a species, or all profiles when speciesID is empty.
func (r *SpeciesProfileRepo) List(speciesID string) ([]SpeciesProfile, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if speciesID == "" {
		rows, err = r.db.Query(`SELECT ` + speciesProfileCols +
			` FROM species_profiles ORDER BY species_id, stage, name`)
	} else {
		rows, err = r.db.Query(`SELECT `+speciesProfileCols+
			` FROM species_profiles WHERE species_id = ? ORDER BY stage, name`, speciesID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SpeciesProfile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *SpeciesProfileRepo) Get(id int64) (*SpeciesProfile, error) {
	p, err := scanProfile(r.db.QueryRow(`SELECT `+speciesProfileCols+
		` FROM species_profiles WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Insert creates a profile and returns its new id.
func (r *SpeciesProfileRepo) Insert(p SpeciesProfile) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO species_profiles (species_id, stage, name, settings_json)
		 VALUES (?, ?, ?, ?)`,
		p.SpeciesID, p.Stage, p.Name, p.SettingsJSON)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *SpeciesProfileRepo) Update(p SpeciesProfile) error {
	_, err := r.db.Exec(
		`UPDATE species_profiles
		    SET species_id = ?, stage = ?, name = ?, settings_json = ?, updated_at = CURRENT_TIMESTAMP
		  WHERE id = ?`,
		p.SpeciesID, p.Stage, p.Name, p.SettingsJSON, p.ID)
	return err
}

func (r *SpeciesProfileRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM species_profiles WHERE id = ?`, id)
	return err
}
