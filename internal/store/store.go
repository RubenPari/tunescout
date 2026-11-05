// Package store persiste libreria, profilo e storico suggerimenti in SQLite.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS known_tracks (
	key        TEXT PRIMARY KEY,
	spotify_id TEXT,
	artist     TEXT NOT NULL,
	title      TEXT NOT NULL,
	source     TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS known_artists (
	norm TEXT PRIMARY KEY,
	name TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS suggested (
	key        TEXT PRIMARY KEY,
	spotify_id TEXT,
	uri        TEXT,
	artist     TEXT NOT NULL,
	title      TEXT NOT NULL,
	album      TEXT,
	year       TEXT,
	reason     TEXT,
	url        TEXT,
	created_at INTEGER NOT NULL,
	feedback   TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS removed (
	key        TEXT PRIMARY KEY,
	spotify_id TEXT,
	artist     TEXT NOT NULL,
	title      TEXT NOT NULL,
	removed_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);`

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

type KnownTrack struct {
	Key, SpotifyID, Artist, Title, Source string
}

// ReplaceLibrary sostituisce atomicamente brani e artisti noti.
func (s *Store) ReplaceLibrary(tracks []KnownTrack, artists map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM known_tracks; DELETE FROM known_artists;`); err != nil {
		return err
	}
	st, err := tx.Prepare(`INSERT OR IGNORE INTO known_tracks(key, spotify_id, artist, title, source) VALUES (?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, t := range tracks {
		if _, err := st.Exec(t.Key, t.SpotifyID, t.Artist, t.Title, t.Source); err != nil {
			return err
		}
	}
	sa, err := tx.Prepare(`INSERT OR IGNORE INTO known_artists(norm, name) VALUES (?,?)`)
	if err != nil {
		return err
	}
	defer sa.Close()
	for n, name := range artists {
		if _, err := sa.Exec(n, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) setMeta(tx interface {
	Exec(string, ...any) (sql.Result, error)
}, k, v string) error {
	_, err := tx.Exec(`INSERT INTO meta(key, value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v)
	return err
}

func (s *Store) SetMetaJSON(k string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.setMeta(s.db, k, string(b))
}

// GetMetaJSON restituisce false se la chiave non esiste.
func (s *Store) GetMetaJSON(k string, v any) (bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, k).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), v)
}

func (s *Store) LastSync() (time.Time, error) {
	var ts int64
	ok, err := s.GetMetaJSON("synced_at", &ts)
	if !ok || err != nil {
		return time.Time{}, err
	}
	return time.Unix(ts, 0), nil
}

func (s *Store) MarkSynced() error { return s.SetMetaJSON("synced_at", time.Now().Unix()) }

func (s *Store) CountKnown() (tracks, artists int, err error) {
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM known_tracks`).Scan(&tracks); err != nil {
		return
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM known_artists`).Scan(&artists)
	return
}

// Seen indica se un brano è già in libreria, è già stato suggerito o è stato
// rimosso dalla libreria con `prune --remove`.
func (s *Store) Seen(key, spotifyID string) (bool, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM known_tracks WHERE key=?1 OR (?2 <> '' AND spotify_id=?2))
		     + (SELECT COUNT(*) FROM suggested    WHERE key=?1 OR (?2 <> '' AND spotify_id=?2))
		     + (SELECT COUNT(*) FROM removed      WHERE key=?1 OR (?2 <> '' AND spotify_id=?2))`,
		key, spotifyID).Scan(&n)
	return n > 0, err
}

// AddRemoved registra i brani rimossi dalla libreria, così discover non li ripropone.
func (s *Store) AddRemoved(tracks []KnownTrack) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT INTO removed(key, spotify_id, artist, title, removed_at) VALUES (?,?,?,?,?)
		ON CONFLICT(key) DO UPDATE SET spotify_id=excluded.spotify_id, removed_at=excluded.removed_at`)
	if err != nil {
		return err
	}
	defer st.Close()
	now := time.Now().Unix()
	for _, t := range tracks {
		if _, err := st.Exec(t.Key, t.SpotifyID, t.Artist, t.Title, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) KnownArtist(normName string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM known_artists WHERE norm=?`, normName).Scan(&n)
	return n > 0, err
}

type Suggestion struct {
	Key, SpotifyID, URI, Artist, Title, Album, Year, Reason, URL string
	CreatedAt                                                    time.Time
	Feedback                                                     string // "", "like", "skip"
}

func (s *Store) AddSuggestions(list []Suggestion) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT OR IGNORE INTO suggested(key, spotify_id, uri, artist, title, album, year, reason, url, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	now := time.Now().Unix()
	for _, x := range list {
		if _, err := st.Exec(x.Key, x.SpotifyID, x.URI, x.Artist, x.Title, x.Album, x.Year, x.Reason, x.URL, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetFeedback(key, fb string) error {
	_, err := s.db.Exec(`UPDATE suggested SET feedback=? WHERE key=?`, fb, key)
	return err
}

// History restituisce i suggerimenti più recenti (limit 0 = tutti).
func (s *Store) History(limit int, feedback string) ([]Suggestion, error) {
	q := `SELECT key, spotify_id, uri, artist, title, album, year, reason, url, created_at, feedback FROM suggested`
	var args []any
	if feedback != "" {
		q += ` WHERE feedback=?`
		args = append(args, feedback)
	}
	q += ` ORDER BY created_at DESC, rowid DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Suggestion
	for rows.Next() {
		var x Suggestion
		var ts int64
		if err := rows.Scan(&x.Key, &x.SpotifyID, &x.URI, &x.Artist, &x.Title, &x.Album, &x.Year, &x.Reason, &x.URL, &ts, &x.Feedback); err != nil {
			return nil, err
		}
		x.CreatedAt = time.Unix(ts, 0)
		out = append(out, x)
	}
	return out, rows.Err()
}
