package main

import (
	"database/sql"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Pitch struct {
	ID         string
	Title      string
	Author     string
	Markdown   string
	HTML       string
	Views      int64
	Created    int64
	Expires    int64
	SecretHash string
}

type Annotation struct {
	ID        int64  `json:"id"`
	PitchID   string `json:"pitch_id"`
	Revision  int    `json:"revision"`
	Session   string `json:"-"`
	Author    string `json:"author"`
	Comment   string `json:"comment"`
	Quote     string `json:"quote"`
	TextStart int    `json:"text_start"`
	TextEnd   int    `json:"text_end"`
	Created   int64  `json:"created"`
	Editable  bool   `json:"editable"`
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

		CREATE TABLE IF NOT EXISTS annotations (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			pitch_id   TEXT NOT NULL REFERENCES pitches(id) ON DELETE CASCADE,
			session    TEXT NOT NULL DEFAULT '',
			author     TEXT NOT NULL DEFAULT '',
			comment    TEXT NOT NULL,
			quote      TEXT NOT NULL,
			text_start INTEGER NOT NULL,
			text_end   INTEGER NOT NULL,
			created    INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_annotations_pitch ON annotations(pitch_id);

		CREATE TABLE IF NOT EXISTS used_stamps (
			hash    TEXT PRIMARY KEY,
			created INTEGER NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	// Add secret_hash column if missing (migration for existing DBs)
	_, err = db.Exec(`ALTER TABLE pitches ADD COLUMN secret_hash TEXT NOT NULL DEFAULT ''`)
	if err != nil && !isAlreadyExists(err) {
		return err
	}

	_, err = db.Exec(`ALTER TABLE annotations ADD COLUMN revision INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !isAlreadyExists(err) {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS revisions (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			pitch_id TEXT NOT NULL REFERENCES pitches(id) ON DELETE CASCADE,
			revision INTEGER NOT NULL,
			title    TEXT NOT NULL DEFAULT '',
			author   TEXT NOT NULL DEFAULT '',
			markdown TEXT NOT NULL,
			html     TEXT NOT NULL,
			created  INTEGER NOT NULL,
			UNIQUE(pitch_id, revision)
		);
		CREATE INDEX IF NOT EXISTS idx_revisions_pitch ON revisions(pitch_id);
	`)
	if err != nil {
		return err
	}

	return nil
}

func isAlreadyExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column")
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) InsertPitch(p *Pitch) error {
	_, err := s.db.Exec(
		`INSERT INTO pitches (id, title, author, markdown, html, views, created, expires, secret_hash)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		p.ID, p.Title, p.Author, p.Markdown, p.HTML, p.Created, p.Expires, p.SecretHash,
	)
	return err
}

func (s *Store) GetPitch(id string) (*Pitch, error) {
	p := &Pitch{}
	err := s.db.QueryRow(
		`SELECT id, title, author, markdown, html, views, created, expires, secret_hash FROM pitches WHERE id = ?`, id,
	).Scan(&p.ID, &p.Title, &p.Author, &p.Markdown, &p.HTML, &p.Views, &p.Created, &p.Expires, &p.SecretHash)
	if err != nil {
		return nil, err
	}
	return p, nil
}

type Revision struct {
	PitchID  string
	Revision int
	Title    string
	Author   string
	Markdown string
	HTML     string
	Created  int64
}

func (s *Store) InsertRevision(r *Revision) error {
	_, err := s.db.Exec(
		`INSERT INTO revisions (pitch_id, revision, title, author, markdown, html, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.PitchID, r.Revision, r.Title, r.Author, r.Markdown, r.HTML, r.Created,
	)
	return err
}

func (s *Store) GetRevision(pitchID string, rev int) (*Revision, error) {
	r := &Revision{}
	err := s.db.QueryRow(
		`SELECT pitch_id, revision, title, author, markdown, html, created
		 FROM revisions WHERE pitch_id = ? AND revision = ?`, pitchID, rev,
	).Scan(&r.PitchID, &r.Revision, &r.Title, &r.Author, &r.Markdown, &r.HTML, &r.Created)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) GetRevisionCount(pitchID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM revisions WHERE pitch_id = ?`, pitchID).Scan(&count)
	return count, err
}

func (s *Store) UpdatePitch(p *Pitch) error {
	_, err := s.db.Exec(
		`UPDATE pitches SET title = ?, author = ?, markdown = ?, html = ?, expires = ? WHERE id = ?`,
		p.Title, p.Author, p.Markdown, p.HTML, p.Expires, p.ID,
	)
	return err
}

func (s *Store) DeletePitch(id string) error {
	_, err := s.db.Exec(`DELETE FROM pitches WHERE id = ?`, id)
	return err
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

func (s *Store) InsertAnnotation(a *Annotation) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO annotations (pitch_id, revision, session, author, comment, quote, text_start, text_end, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.PitchID, a.Revision, a.Session, a.Author, a.Comment, a.Quote, a.TextStart, a.TextEnd, a.Created,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetAnnotations(pitchID string, revision int) ([]Annotation, error) {
	rows, err := s.db.Query(
		`SELECT id, pitch_id, revision, session, author, comment, quote, text_start, text_end, created
		 FROM annotations WHERE pitch_id = ? AND revision = ? ORDER BY text_start, created`, pitchID, revision,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Annotation
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.ID, &a.PitchID, &a.Revision, &a.Session, &a.Author, &a.Comment, &a.Quote, &a.TextStart, &a.TextEnd, &a.Created); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAnnotation(id int64) (*Annotation, error) {
	a := &Annotation{}
	err := s.db.QueryRow(
		`SELECT id, pitch_id, session, author, comment, quote, text_start, text_end, created
		 FROM annotations WHERE id = ?`, id,
	).Scan(&a.ID, &a.PitchID, &a.Session, &a.Author, &a.Comment, &a.Quote, &a.TextStart, &a.TextEnd, &a.Created)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) ReassignAnnotations(pitchID string, fromRevision, toRevision int) error {
	_, err := s.db.Exec(
		`UPDATE annotations SET revision = ? WHERE pitch_id = ? AND revision = ?`,
		toRevision, pitchID, fromRevision,
	)
	return err
}

func (s *Store) UpdateAnnotation(id int64, author, comment string) error {
	_, err := s.db.Exec(`UPDATE annotations SET author = ?, comment = ? WHERE id = ?`, author, comment, id)
	return err
}

func (s *Store) DeleteAnnotation(id int64) error {
	_, err := s.db.Exec(`DELETE FROM annotations WHERE id = ?`, id)
	return err
}

func (s *Store) UpdateAuthorBySession(pitchID, session, author string) error {
	_, err := s.db.Exec(`UPDATE annotations SET author = ? WHERE pitch_id = ? AND session = ?`, author, pitchID, session)
	return err
}
