package main

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Pitch struct {
	ID       string
	Title    string
	Author   string
	Markdown string
	HTML     string
	Views    int64
	Created  int64
	Expires  int64
}

func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pitches (
			id       TEXT PRIMARY KEY,
			title    TEXT NOT NULL DEFAULT '',
			author   TEXT NOT NULL DEFAULT '',
			markdown TEXT NOT NULL,
			html     TEXT NOT NULL,
			views    INTEGER NOT NULL DEFAULT 0,
			created  INTEGER NOT NULL,
			expires  INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_pitches_expires ON pitches(expires) WHERE expires > 0;

		CREATE TABLE IF NOT EXISTS used_stamps (
			hash    TEXT PRIMARY KEY,
			created INTEGER NOT NULL
		);
	`)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) InsertPitch(p *Pitch) error {
	_, err := s.db.Exec(
		`INSERT INTO pitches (id, title, author, markdown, html, views, created, expires)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		p.ID, p.Title, p.Author, p.Markdown, p.HTML, p.Created, p.Expires,
	)
	return err
}

func (s *Store) GetPitch(id string) (*Pitch, error) {
	p := &Pitch{}
	err := s.db.QueryRow(
		`SELECT id, title, author, markdown, html, views, created, expires FROM pitches WHERE id = ?`, id,
	).Scan(&p.ID, &p.Title, &p.Author, &p.Markdown, &p.HTML, &p.Views, &p.Created, &p.Expires)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) IncrementViews(id string) error {
	_, err := s.db.Exec(`UPDATE pitches SET views = views + 1 WHERE id = ?`, id)
	return err
}

// UseStamp records a PoW stamp hash to prevent replay. Returns false if already used.
func (s *Store) UseStamp(hash string) (bool, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO used_stamps (hash, created) VALUES (?, ?)`,
		hash, time.Now().Unix(),
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *Store) CleanExpired() (pitches int64, stamps int64, err error) {
	now := time.Now().Unix()

	res, err := s.db.Exec(`DELETE FROM pitches WHERE expires > 0 AND expires < ?`, now)
	if err != nil {
		return 0, 0, err
	}
	pitches, _ = res.RowsAffected()

	stampExpiry := now - 600 // 10 minutes
	res, err = s.db.Exec(`DELETE FROM used_stamps WHERE created < ?`, stampExpiry)
	if err != nil {
		return pitches, 0, err
	}
	stamps, _ = res.RowsAffected()

	return pitches, stamps, nil
}

func (s *Store) PitchExists(id string) bool {
	var exists int
	s.db.QueryRow(`SELECT 1 FROM pitches WHERE id = ? LIMIT 1`, id).Scan(&exists)
	return exists == 1
}
