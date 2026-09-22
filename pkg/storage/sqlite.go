package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // SQLite driver
)

const createSignalTable = `CREATE TABLE IF NOT EXISTS signal (
	name TEXT PRIMARY KEY NOT NULL,
	notifier TEXT NULL,
	next_signal DATETIME NULL,
	all_clear INTEGER DEFAULT 0 NULL,
	meta TEXT NULL
)`

type sqliteDB struct {
	db *sql.DB
}

// NewSQLiteDB creates new Storage using sqlite backend.
func NewSQLiteDB(dsn string) (Storage, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("unable to open sqlite database: %w", err)
	}
	if _, err := db.Exec(createSignalTable); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("unable to create sqlite table: %w", err)
	}
	if err := validateSignalSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("unable to validate sqlite table: %w", err)
	}

	return &sqliteDB{db: db}, nil
}

func validateSignalSchema(db *sql.DB) (err error) {
	rows, err := db.Query("SELECT name, notifier, next_signal, all_clear, meta FROM signal LIMIT 0")
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	return nil
}

func (d *sqliteDB) Close() error {
	return d.db.Close()
}

func (d *sqliteDB) Load() (signals []Signal, err error) {
	rows, err := d.db.Query("SELECT name, notifier, CAST(next_signal AS TEXT), all_clear, meta FROM signal")
	if err != nil {
		return nil, fmt.Errorf("unable to load signals from sqlite: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("unable to close sqlite rows: %w", closeErr)
		}
	}()

	for rows.Next() {
		var (
			signal     Signal
			nextSignal string
			meta       []byte
		)
		if err := rows.Scan(&signal.Name, &signal.Notifier, &nextSignal, &signal.AllClear, &meta); err != nil {
			return nil, fmt.Errorf("unable to scan sqlite signal: %w", err)
		}

		signal.NextSignal, err = parseSQLiteTime(nextSignal)
		if err != nil {
			return nil, fmt.Errorf("unable to parse next signal for %q: %w", signal.Name, err)
		}
		if len(meta) > 0 {
			if err := json.Unmarshal(meta, &signal.Meta); err != nil {
				return nil, fmt.Errorf("unable to decode metadata for %q: %w", signal.Name, err)
			}
		}
		signals = append(signals, signal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("unable to iterate sqlite signals: %w", err)
	}

	return signals, nil
}

func (d *sqliteDB) Save(s Signal) error {
	meta, err := json.Marshal(s.Meta)
	if err != nil {
		return fmt.Errorf("unable to jsonify signal metadata: %w", err)
	}

	_, err = d.db.Exec(
		"INSERT OR REPLACE INTO signal (name, notifier, next_signal, all_clear, meta) VALUES (?, ?, ?, ?, ?)",
		s.Name,
		s.Notifier,
		s.NextSignal.UTC().Format(time.RFC3339Nano),
		s.AllClear,
		meta,
	)
	if err != nil {
		return fmt.Errorf("unable to save signal to sqlite: %w", err)
	}
	return nil
}

func (d *sqliteDB) Remove(s Signal) error {
	if s.Name == "" {
		return nil
	}
	if _, err := d.db.Exec("DELETE FROM signal WHERE name = ?", s.Name); err != nil {
		return fmt.Errorf("unable to remove sqlite record: %w", err)
	}
	return nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported sqlite timestamp %q", value)
}
